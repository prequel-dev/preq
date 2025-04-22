package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prequel-dev/preq/internal/pkg/resolve"
	"github.com/prequel-dev/preq/internal/pkg/utils"
	"github.com/prequel-dev/preq/internal/pkg/ux"
	"github.com/prequel-dev/prequel-compiler/pkg/compiler"
	"github.com/prequel-dev/prequel-compiler/pkg/matchz"
	"github.com/prequel-dev/prequel-compiler/pkg/parser"
	"github.com/prequel-dev/prequel-compiler/pkg/pqerr"
	"github.com/prequel-dev/prequel-compiler/pkg/schema"
	"github.com/prequel-dev/prequel-logmatch/pkg/entry"
	lm "github.com/prequel-dev/prequel-logmatch/pkg/match"
	"github.com/prequel-dev/prequel-logmatch/pkg/scanner"

	"github.com/jedib0t/go-pretty/v6/progress"
	"github.com/rs/zerolog/log"
)

type LogData = resolve.LogData

const ramLimit = 512 << 20 // 512 MiB

var (
	ErrRuleNotFound      = errors.New("rule not found")
	ErrUnknownObjectType = errors.New("unknown object type")
	ErrExpectedMatcherCb = errors.New("expected matcher callback")
	ErrDuplicateRule     = errors.New("duplicate rule")
)

type RuntimeT struct {
	mux   sync.RWMutex
	Stop  int64
	Ux    ux.UxFactoryI
	Rules map[string]parser.ParseCreT
}

func New(stop int64, ux ux.UxFactoryI) *RuntimeT {
	return &RuntimeT{
		Stop:  stop,
		Rules: make(map[string]parser.ParseCreT),
		Ux:    ux,
	}
}

func (r *RuntimeT) Close() error {
	return nil
}

func GetEventSource(obj *compiler.ObjT) parser.ParseEventT {

	return parser.ParseEventT{
		Source: obj.Event.Source,
	}
}

func compileRuleTree(cf compiler.RuntimeI, tree *parser.TreeT) (compiler.ObjsT, error) {
	var (
		err      error
		nodeObjs compiler.ObjsT
	)

	opts := []compiler.CompilerOptT{
		compiler.WithRuntime(cf),
	}

	if nodeObjs, err = compiler.CompileTree(tree, schema.ScopeNode, opts...); err != nil {
		return nil, err
	}

	return nodeObjs, nil
}

func compileRulePath(cf compiler.RuntimeI, path string) (compiler.ObjsT, *parser.RulesT, error) {
	var (
		rules    *parser.RulesT
		tree     *parser.TreeT
		nodeObjs compiler.ObjsT
		err      error
	)

	log.Info().Str("path", path).Msg("Parsing rules")

	if rules, err = utils.ParseRulesPath(path); err != nil {
		log.Error().Err(err).Msg("Failed to parse rules")
		return nil, nil, err
	}

	if tree, err = parser.ParseRules(rules); err != nil {
		return nil, nil, err
	}

	log.Info().Int("cres", len(rules.Rules)).Msg("Parsed rules")
	for _, rule := range rules.Rules {
		log.Info().Str("id", rule.Metadata.Id).Str("cre", rule.Cre.Id).Msg("Rule")
	}

	nodeObjs, err = compileRuleTree(cf, tree)
	if err != nil {
		return nil, nil, pqerr.WithFile(err, path)
	}

	return nodeObjs, rules, nil
}

func compileRule(cf compiler.RuntimeI, data []byte) (compiler.ObjsT, *parser.RulesT, error) {
	var (
		rules    *parser.RulesT
		tree     *parser.TreeT
		nodeObjs compiler.ObjsT
		err      error
	)

	if rules, err = utils.ParseRules(bytes.NewReader(data)); err != nil {
		log.Error().Err(err).Msg("Failed to parse rules")
		return nil, nil, err
	}

	if tree, err = parser.ParseRules(rules); err != nil {
		return nil, nil, err
	}

	log.Info().Int("cres", len(rules.Rules)).Msg("Parsed rules")
	for _, rule := range rules.Rules {
		log.Info().Str("id", rule.Metadata.Id).Str("cre", rule.Cre.Id).Msg("Rule")
	}

	nodeObjs, err = compileRuleTree(cf, tree)
	if err != nil {
		log.Error().Err(err).Msg("Failed to compile rule tree")
		return nil, nil, err
	}

	return nodeObjs, rules, nil
}

func (r *RuntimeT) compileRules(cf compiler.RuntimeI, data []byte) (compiler.ObjsT, *parser.RulesT, error) {

	var (
		rules *parser.RulesT
		err   error
	)

	var (
		nObjs compiler.ObjsT
		ok    bool
	)

	if nObjs, rules, err = compileRule(cf, data); err != nil {
		return nil, nil, err
	}

	r.Ux.IncrementRuleTracker(int64(len(rules.Rules)))

	if ok, err = validateRules(rules, nil); !ok {
		return nil, nil, err
	}

	return nObjs, rules, nil

}

func (r *RuntimeT) compileRulesPaths(cf compiler.RuntimeI, paths []string) (compiler.ObjsT, []*parser.RulesT, error) {
	var (
		nodeObjs = make(compiler.ObjsT, 0)
		allRules = make([]*parser.RulesT, 0)

		err error
	)

	for _, path := range paths {

		var (
			nObjs compiler.ObjsT
			rules *parser.RulesT
			ok    bool
		)

		if nObjs, rules, err = compileRulePath(cf, path); err != nil {
			return nil, nil, err
		}

		r.Ux.IncrementRuleTracker(int64(len(rules.Rules)))

		if ok, err = validateRules(rules, allRules); !ok {
			return nil, nil, err
		}

		nodeObjs = append(nodeObjs, nObjs...)

		allRules = append(allRules, rules)
	}

	return nodeObjs, allRules, nil
}

func validateRule(rule parser.ParseRuleT, dupes map[string]struct{}) (bool, error) {

	if _, ok := dupes[rule.Metadata.Id]; ok {
		log.Error().Str("id", rule.Metadata.Id).Msg("Duplicate rule hash id. Aborting...")
		return false, fmt.Errorf("duplicate rule hash id=%s cre=%s", rule.Metadata.Id, rule.Cre.Id)
	}

	if _, ok := dupes[rule.Metadata.Hash]; ok {
		log.Error().Str("id", rule.Metadata.Hash).Msg("Duplicate rule hash id. Aborting...")
		return false, fmt.Errorf("duplicate rule hash id=%s cre=%s", rule.Metadata.Hash, rule.Cre.Id)
	}

	if _, ok := dupes[rule.Cre.Id]; ok {
		log.Error().Str("id", rule.Cre.Id).Msg("Duplicate rule hash id. Aborting...")
		return false, fmt.Errorf("duplicate rule hash id=%s cre=%s", rule.Cre.Id, rule.Cre.Id)
	}

	dupes[rule.Metadata.Id] = struct{}{}
	dupes[rule.Metadata.Hash] = struct{}{}
	dupes[rule.Cre.Id] = struct{}{}

	return true, nil
}

func validateRules(rules *parser.RulesT, allRules []*parser.RulesT) (bool, error) {

	var (
		dupes = make(map[string]struct{})
		ok    bool
		err   error
	)

	// Check for dupes with other rules in the same config
	for _, rule := range rules.Rules {
		if ok, err = validateRule(rule, dupes); !ok {
			return false, err
		}
	}

	// Check for global dupes. Skip if nil.
	for _, config := range allRules {
		for _, rule := range config.Rules {
			if ok, err = validateRule(rule, dupes); !ok {
				return false, err
			}
		}
	}

	return true, nil
}

type RuleMatchersT struct {
	match    map[string]any
	cb       map[string]compiler.CbMatchT
	eventSrc map[string]parser.ParseEventT
}

func (r *RuntimeT) AddRules(rules *parser.RulesT) error {
	r.mux.Lock()
	defer r.mux.Unlock()

	var ok bool
	for _, rule := range rules.Rules {
		if _, ok = r.Rules[rule.Metadata.Hash]; !ok {
			r.Rules[rule.Metadata.Hash] = rule.Cre
		} else {
			log.Error().Str("ruleHash", rule.Metadata.Hash).Msg("Duplicate rule")
			return ErrDuplicateRule
		}
	}

	return nil
}

func loadNodeObjs(objs compiler.ObjsT) (*RuleMatchersT, error) {
	var (
		m = &RuleMatchersT{
			match:    make(map[string]any),
			cb:       make(map[string]compiler.CbMatchT),
			eventSrc: make(map[string]parser.ParseEventT),
		}
	)

	for _, obj := range objs {

		var ok bool
		if m.cb[obj.RuleId], ok = obj.Cb.Callback.(compiler.CbMatchT); !ok {
			return nil, ErrExpectedMatcherCb
		}

		switch obj.AbstractType {
		case schema.NodeTypeLogSeq:

			switch o := obj.Object.(type) {
			case *lm.InverseSeq, *lm.MatchSeq:
				m.match[obj.RuleId] = o
			default:
				log.Error().
					Str("rule_id", obj.RuleId).
					Type("type", o).
					Msg("Unexpected matcher type")
				return nil, ErrUnknownObjectType
			}

		case schema.NodeTypeLogSet:
			switch o := obj.Object.(type) {
			case *lm.MatchSingle, *lm.MatchSet, *lm.MatchFunc, *lm.InverseSet:
				m.match[obj.RuleId] = o
			default:
				log.Error().
					Str("rule_id", obj.RuleId).
					Type("type", o).
					Msg("Unexpected matcher type")
				return nil, ErrUnknownObjectType
			}

		default:
			log.Error().
				Str("rule_id", obj.RuleId).
				Str("abstract_type", obj.AbstractType.String()).
				Any("obj", obj).
				Msg("Unknown object type")
			return nil, ErrUnknownObjectType
		}

		m.eventSrc[obj.RuleId] = GetEventSource(obj)
	}

	return m, nil
}

func (r *RuntimeT) getCre(ruleHash string) (parser.ParseCreT, error) {
	r.mux.RLock()
	defer r.mux.RUnlock()

	cre, ok := r.Rules[ruleHash]
	if !ok {
		return parser.ParseCreT{}, ErrRuleNotFound
	}

	return cre, nil
}

type runtimeCb func(params compiler.MatchParamsT, m matchz.HitsT) error

type runtimeT struct {
	cb runtimeCb
}

func NewRuntime(cb runtimeCb) *runtimeT {
	return &runtimeT{
		cb: cb,
	}
}

func (r *runtimeT) NewCbMatch(params compiler.MatchParamsT) compiler.CbMatchT {

	return func(ctx context.Context, m matchz.HitsT) error {
		m.Entity.Origin = params.Origin
		return r.cb(params, m)
	}

}

func (r *runtimeT) NewCbAssert(params compiler.AssertParamsT) compiler.CbAssertT {
	return func(ctx context.Context) error {
		return nil
	}
}

func (r *runtimeT) LoadAssertObject(ctx context.Context, obj *compiler.ObjT) error {
	return nil
}

func (r *runtimeT) LoadMachineObject(ctx context.Context, obj *compiler.ObjT, userCb any) error {
	return nil
}

// Permit wasm compile rules from a byte slice
func (r *RuntimeT) CompileRules(ruleData []byte, report *ux.ReportT) (*RuleMatchersT, error) {
	var (
		nodeObjs compiler.ObjsT
		rules    *parser.RulesT
		err      error
		matchers *RuleMatchersT
	)

	runtime := r.getRuntimeCb(report)

	if nodeObjs, rules, err = r.compileRules(runtime, ruleData); err != nil {
		log.Error().Err(err).Msg("Failed to load rules")
		return nil, err
	}
	r.Ux.MarkRuleTrackerDone()

	// Rules are validated
	r.AddRules(rules)
	report.AddRules(rules)

	if matchers, err = loadNodeObjs(nodeObjs); err != nil {
		log.Error().Err(err).Msg("Failed to load node objects")
	}

	return matchers, nil
}

func (r *RuntimeT) getRuntimeCb(report *ux.ReportT) *runtimeT {
	var err error
	runtime := NewRuntime(func(params compiler.MatchParamsT, m matchz.HitsT) error {

		var (
			cre      parser.ParseCreT
			ruleHash = params.Address.GetRuleHash()
		)

		if cre, err = r.getCre(ruleHash); err != nil {
			log.Error().Str("rule_hash", ruleHash).Msg("Failed to get CRE for rule")
			return err
		}

		var (
			ts time.Time
			ok bool
		)

		if m.Entity.Origin {

			log.Warn().
				Interface("origin", m.Entity).
				Msg("Origin match")

			ts = time.Unix(0, m.Entries[0].Timestamp)

			for _, hit := range m.Entries {
				log.Warn().
					Interface("hit", string(hit.Entry)).
					Int64("timestamp", hit.Timestamp).
					Msg("Entry")
			}

		} else {
			log.Warn().
				Interface("related", m.Entity).
				Msg("Related match")
		}

		if ok = report.AddCreHit(&cre, ts, m); ok {
			r.Ux.IncrementProblemsTracker(1)
		}

		return nil
	})

	return runtime
}

func (r *RuntimeT) CompileRulesPath(rulesPaths []string, report *ux.ReportT) (*RuleMatchersT, error) {

	var (
		nodeObjs compiler.ObjsT
		configs  []*parser.RulesT
		err      error
		matchers *RuleMatchersT
	)

	runtime := r.getRuntimeCb(report)

	if nodeObjs, configs, err = r.compileRulesPaths(runtime, rulesPaths); err != nil {
		return nil, err
	}

	r.Ux.MarkRuleTrackerDone()

	// Rules are validated
	for _, rules := range configs {
		r.AddRules(rules)
		report.AddRules(rules)
	}

	if matchers, err = loadNodeObjs(nodeObjs); err != nil {
		log.Error().Err(err).Msg("Failed to load node objects")
		return nil, err
	}

	return matchers, nil
}

func (r *RuntimeT) LoadRulesPaths(rep *ux.ReportT, rulesPaths []string) (*RuleMatchersT, error) {

	var (
		ruleMatchers *RuleMatchersT
		err          error
	)

	if ruleMatchers, err = r.CompileRulesPath(rulesPaths, rep); err != nil {
		return nil, err
	}

	return ruleMatchers, nil
}

func (r *RuntimeT) Run(ctx context.Context, ruleMatchers *RuleMatchersT, sources []*LogData, report *ux.ReportT) error {

	var (
		wg    sync.WaitGroup
		lines atomic.Int64
		err   error
	)

	err = r._run(ctx, &wg, sources, ruleMatchers, r.Stop, &lines)
	if err != nil {
		log.Error().Err(err).Msg("Failed to run input")
		return err
	}

	killCh := make(chan struct{})
	defer close(killCh)

	r.Ux.StartLinesTracker(&lines, killCh)

	r.Ux.StartProblemsTracker()
	defer r.Ux.MarkProblemsTrackerDone()

	wg.Wait()

	return err
}

func (r *RuntimeT) _run(ctx context.Context, wg *sync.WaitGroup, sources []*LogData, matchers *RuleMatchersT, stop int64, lines *atomic.Int64) error {

	var dupeMap = make(map[string]struct{}, len(sources))

	for _, logData := range sources {

		// Currently supports one data source per type; warn on dupe and continue.
		if _, ok := dupeMap[logData.SrcType()]; ok {
			log.Warn().
				Str("name", logData.Name()).
				Str("src", logData.SrcType()).
				Msg("Ignore duplicate source")
			continue
		}
		dupeMap[logData.SrcType()] = struct{}{}

		if err := r._runSrc(ctx, wg, logData, matchers, stop, lines); err != nil {
			return err
		}
	}

	return nil
}

func (r *RuntimeT) _runSrc(ctx context.Context, wg *sync.WaitGroup, ld *LogData, matchers *RuleMatchersT, stop int64, lines *atomic.Int64) error {

	type pairT struct {
		matcher    matchCB
		compilerCb compiler.CbMatchT
	}

	var (
		srcType = ld.SrcType()
		cbs     = make([]pairT, 0, len(matchers.eventSrc))
	)

	for ruleId, pe := range matchers.eventSrc {

		if srcType != "*" && srcType != pe.Source {
			continue
		}

		log.Info().
			Str("src", ld.Name()).
			Str("srcType", srcType).
			Str("ruleId", ruleId).
			Msg("Matching source")

		cb, err := _makeMatchCb(srcType, matchers.match[ruleId])
		if err != nil {
			log.Error().Err(err).Msg("Failed to make stdin callback")
			return err
		}

		cbs = append(cbs, pairT{
			matcher:    cb,
			compilerCb: matchers.cb[ruleId],
		})
	}

	if len(cbs) == 0 {
		log.Info().Str("src", srcType).Msg("No matchers found")
		return nil
	}

	name := ld.Name()
	if name == "" {
		name = ld.SrcType()
	}

	tracker, err := r.Ux.NewBytesTracker(name)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create bytes tracker. Continue...")
	}

	if total := ld.Size(); total > 0 {
		tracker.UpdateTotal(total)
	}

	scanCb := func(entry entry.LogEntry) bool {

		// Use an atomic instead of calling tracker directly to decrease overhead.
		lines.Add(1)

		for _, pair := range cbs {
			if msgHits := pair.matcher(entry); msgHits != nil {
				log.Info().
					Interface("hits", msgHits).
					Msg("Hits")
				pair.compilerCb(ctx, *msgHits)
			}
		}

		return false
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		_spinLogs(ld, scanCb, stop, tracker)
		tracker.MarkAsDone()
	}()

	return nil
}

func _spinLogs(ld *LogData, scanF scanner.ScanFuncT, stop int64, tracker *progress.Tracker) {

	for i, rd := range ld.Logs {

		trdr := &TrkRdr{
			rd:  rd,
			trk: tracker,
		}

		log.Info().
			Int("i", i).
			Int("n", len(ld.Logs)).
			Str("name", rd.Name()).
			Int64("size", rd.Size()).
			Msg("Scanning log")

		opts := []scanner.ScanOptT{
			scanner.WithStop(stop),
		}

		if rd.Fold() {
			opts = append(opts, scanner.WithFold(true))
		}

		// If reorder is enabled, hook the middleware.
		var reorder *scanner.ReorderT
		if rd.Window() > 0 {
			var err error
			if reorder, err = scanner.NewReorder(rd.Window(), scanF, scanner.WithMemoryLimit(ramLimit)); err != nil {
				log.Warn().Err(err).Msg("Fail to create reorder object. Continue...")
			} else {
				scanF = reorder.Append
			}
		}

		parser := rd.Parser()
		err := scanner.ScanForward(
			trdr,
			parser.ReadEntry,
			scanF,
			opts...,
		)

		switch {
		case err != nil:
			log.Warn().
				Err(err).
				Str("name", rd.Name()).
				Int64("size", rd.Size()).
				Msg("Failed to scan log.  Continue...")
		case reorder != nil:
			reorder.Flush()
		}

		rd.Close()
	}
}

type matchCB func(entry entry.LogEntry) *matchz.HitsT

func _makeMatchCb(src string, matcher any) (matchCB, error) {

	mm, ok := matcher.(lm.Matcher)
	if !ok {
		return nil, errors.New("invalid matcher")
	}

	scanCb := func(entry entry.LogEntry) *matchz.HitsT {

		hits := mm.Scan(entry)
		if hits.Cnt == 0 {
			return nil
		}

		log.Trace().Any("hits", hits).Msg("Hits")

		msgHits := matchz.HitsT{
			Entries: make([]matchz.EntryT, 0, len(hits.Logs)),
		}

		for _, line := range hits.Logs {
			msgHits.Entries = append(msgHits.Entries, matchz.EntryT{
				Timestamp: line.Timestamp,
				Entry:     []byte(line.Line),
			})
		}

		msgHits.Count = uint32(hits.Cnt)
		msgHits.Entity.FileName = src

		return &msgHits
	}

	return scanCb, nil
}

type TrkRdr struct {
	rd  io.Reader
	trk *progress.Tracker
}

func (r *TrkRdr) Read(p []byte) (n int, err error) {
	n, err = r.rd.Read(p)
	r.trk.Increment(int64(n))
	return
}
