package parser

import (
	"errors"
	"time"

	"github.com/rs/zerolog/log"
)

type NodeTypeT string

const (
	NodeTypeSeq          NodeTypeT = "seq"
	NodeTypeSeqNeg       NodeTypeT = "seq_neg"
	NodeTypeSeqNegSingle NodeTypeT = "seq_neg_single"
	NodeTypeSet          NodeTypeT = "set"
	NodeTypeSetNeg       NodeTypeT = "set_neg"
	NodeTypeSetNegSingle NodeTypeT = "set_neg_single"
)

var (
	ErrNotSupported = errors.New("not supported")
	ErrTermNotFound = errors.New("term not found")
	ErrMissingOrder = errors.New("sequence missing order")
	ErrMissingMatch = errors.New("set missing match")
	ErrInvalidSet   = errors.New("invalid set")
	ErrInvalidSeq   = errors.New("invalid sequence")
)

type TreeT struct {
	Nodes []*NodeT `json:"nodes"`
}

type EventT struct {
	Origin bool   `json:"origin"`
	Source string `json:"source"`
}

type NodeMetadataT struct {
	RuleHash     string        `json:"rule_hash"`
	RuleId       string        `json:"rule_id"`
	Window       time.Duration `json:"window"`
	Event        *EventT       `json:"event"`
	Type         NodeTypeT     `json:"type"`
	Correlations []string      `json:"correlations"`
	NegateOpts   *NegateOptsT  `json:"negate_opts"`
}

type NodeT struct {
	Metadata NodeMetadataT `json:"metadata"`
	NegIdx   int           `json:"neg_idx"`
	Children []any         `json:"children"`
}

type NegateOptsT struct {
	Window   time.Duration `json:"window"`
	Slide    time.Duration `json:"slide"`
	Anchor   uint32        `json:"anchor"`
	Absolute bool          `json:"absolute"`
}

type FieldT struct {
	Field      string       `json:"field"`
	StrValue   string       `json:"value"`
	JqValue    string       `json:"jq_value"`
	RegexValue string       `json:"regex_value"`
	Count      int          `json:"count"`
	NegateOpts *NegateOptsT `json:"negate"`
}

type TermsT struct {
	Fields []FieldT `json:"fields"`
}

type MatcherT struct {
	Match  TermsT        `json:"match"`
	Negate TermsT        `json:"negate"`
	Window time.Duration `json:"window"`
}

func newEvent(t *ParseEventT) *EventT {
	return &EventT{
		Source: t.Source,
		Origin: t.Origin,
	}
}

func initNode(ruleId, ruleHash string) *NodeT {
	return &NodeT{
		Metadata: NodeMetadataT{
			RuleId:   ruleId,
			RuleHash: ruleHash,
		},
		NegIdx:   -1,
		Children: make([]any, 0),
	}
}

func seqNodeProps(node *NodeT, seq *ParseSequenceT, order, negate bool) error {

	switch {
	case order && negate:
		node.Metadata.Type = NodeTypeSeqNeg
	case order && !negate:
		node.Metadata.Type = NodeTypeSeq
	case !order && negate:
		node.Metadata.Type = NodeTypeSeqNegSingle
	case !order && !negate:
		return ErrMissingOrder
	}

	if seq.Event != nil {
		node.Metadata.Event = newEvent(seq.Event)
	}

	if seq.Window != "" {
		var err error
		if node.Metadata.Window, err = time.ParseDuration(seq.Window); err != nil {
			log.Error().Err(err).Msg("Failed to parse window")
			return err
		}
	}

	if seq.Correlations != nil {
		node.Metadata.Correlations = seq.Correlations
	}

	return nil
}

func setNodeProps(node *NodeT, set *ParseSetT, match, negate bool) error {

	switch {
	case match && negate:
		node.Metadata.Type = NodeTypeSetNeg
	case match && !negate:
		node.Metadata.Type = NodeTypeSet
	case !match && negate:
		node.Metadata.Type = NodeTypeSetNegSingle
	case !match && !negate:
		return ErrMissingMatch
	}

	if set.Event != nil {
		node.Metadata.Event = newEvent(set.Event)
	}

	if set.Window != "" {
		var err error
		if node.Metadata.Window, err = time.ParseDuration(set.Window); err != nil {
			log.Error().Err(err).Msg("Failed to parse window")
			return err
		}
	}

	if set.Correlations != nil {
		node.Metadata.Correlations = set.Correlations
	}

	return nil
}

func buildTree(terms map[string]ParseTermT, r ParseRuleT) (*NodeT, error) {

	var (
		root = initNode(r.Metadata.Id, r.Metadata.Hash)
	)

	switch {
	case r.Rule.Sequence != nil:
		return buildSequenceTree(root, terms, r)
	case r.Rule.Set != nil:
		return buildSetTree(root, terms, r)
	default:
		log.Error().Interface("rule", r).Msg("Unsupported rule type")
		return nil, ErrNotSupported
	}
}

// buildSequenceTree processes a rule with a Sequence definition.
func buildSequenceTree(root *NodeT, terms map[string]ParseTermT, r ParseRuleT) (*NodeT, error) {

	var (
		seq = r.Rule.Sequence
	)

	if seq == nil {
		return nil, ErrInvalidSeq
	}

	// Build positive children from seq.Order (non-negated)
	// Build negative children from seq.Negate (negated)
	pos, neg, err := buildChildrenGroups(root, terms, seq.Order, seq.Negate)
	if err != nil {
		return nil, err
	}

	// Apply sequence-specific node properties
	if err := seqNodeProps(root, seq, seq.Order != nil, seq.Negate != nil); err != nil {
		return nil, err
	}

	// Order positive first, then negatives
	root.Children = append(root.Children, pos...)
	root.Children = append(root.Children, neg...)
	if len(neg) > 0 {
		root.NegIdx = len(pos)
	}

	return root, nil
}

// buildSetTree processes a rule with a Set definition.
func buildSetTree(root *NodeT, terms map[string]ParseTermT, r ParseRuleT) (*NodeT, error) {

	var (
		set = r.Rule.Set
	)

	if set == nil {
		return nil, ErrInvalidSet
	}

	// Notice in the original code, for "Set.Negate" we also passed false
	// to buildChildren, so we do the same here for consistency.
	pos, neg, err := buildChildrenGroups(root, terms, set.Match, set.Negate)
	if err != nil {
		return nil, err
	}

	// Apply set-specific node properties
	if err := setNodeProps(root, set, set.Match != nil, set.Negate != nil); err != nil {
		return nil, err
	}

	// Order positive first, then negatives
	root.Children = append(root.Children, pos...)
	root.Children = append(root.Children, neg...)
	if len(neg) > 0 {
		root.NegIdx = len(pos)
	}

	return root, nil
}

// buildChildrenGroups is a helper for building positive/negative children
// in a single pass. The boolean flags specify whether each slice
// is being treated as negated or not.
func buildChildrenGroups(root *NodeT, terms map[string]ParseTermT, matches, negates []ParseTermT) (pos []any, neg []any, err error) {

	pos = []any{}
	neg = []any{}

	if len(matches) > 0 {

		cPos, err := buildChildren(root, terms, matches, false)
		if err != nil {
			return nil, nil, err
		}
		pos = append(pos, cPos...)
	}

	if len(negates) > 0 {
		log.Debug().
			Msg("Building negate children")

		cNeg, err := buildChildren(root, terms, negates, true)
		if err != nil {
			return nil, nil, err
		}
		// If double-negatives or other logic is needed, adjust the append here
		neg = append(neg, cNeg...)
	}

	return pos, neg, nil
}

func buildChildren(parent *NodeT, tm map[string]ParseTermT, terms []ParseTermT, parentNegate bool) ([]any, error) {
	var (
		children = make([]any, 0)
	)

	for _, term := range terms {
		var (
			node any
			t    = term
			err  error
		)

		if term.StrValue != "" {
			// If the term is not found in the terms map, then use as str value
			if resolvedTerm, ok := tm[term.StrValue]; ok {
				t = resolvedTerm
				if term.NegateOpts != nil {
					t.NegateOpts = term.NegateOpts
				}
			}
		}

		if node, err = nodeFromTerm(parent, tm, t, parentNegate); err != nil {
			log.Error().Err(err).Msg("Failed to build tree")
			return nil, err
		}

		children = append(children, node)

	}

	return children, nil
}

func nodeFromTerm(parent *NodeT, terms map[string]ParseTermT, term ParseTermT, parentNegate bool) (any, error) {

	var (
		node *NodeT
		opts *NegateOptsT
		err  error
	)

	switch {
	case term.Sequence != nil:
		if node, err = buildSequenceNode(parent, terms, term.Sequence); err != nil {
			return nil, err
		}

		if term.NegateOpts != nil {
			if opts, err = negateOpts(term); err != nil {
				return nil, err
			}
			node.Metadata.NegateOpts = opts
		}
	case term.Set != nil:
		if node, err = buildSetNode(parent, terms, term.Set); err != nil {
			return nil, err
		}

		if term.NegateOpts != nil {
			if opts, err = negateOpts(term); err != nil {
				return nil, err
			}
			node.Metadata.NegateOpts = opts
		}
	case term.StrValue != "" || term.JqValue != "" || term.RegexValue != "":
		return parseValue(term, parentNegate)

	default:
		return nil, ErrTermNotFound
	}

	return node, nil
}

func negateOpts(term ParseTermT) (*NegateOptsT, error) {
	var (
		opts = &NegateOptsT{}
		err  error
	)

	if term.NegateOpts.Window != "" {
		if opts.Window, err = time.ParseDuration(term.NegateOpts.Window); err != nil {
			return nil, err
		}
	}

	if term.NegateOpts.Slide != "" {
		if opts.Slide, err = time.ParseDuration(term.NegateOpts.Slide); err != nil {
			return nil, err
		}
	}

	opts.Anchor = term.NegateOpts.Anchor
	opts.Absolute = term.NegateOpts.Absolute

	return opts, nil
}

func buildSequenceNode(parent *NodeT, terms map[string]ParseTermT, seq *ParseSequenceT) (*NodeT, error) {
	node := initNode(parent.Metadata.RuleId, parent.Metadata.RuleHash)

	pos, neg, err := buildPosNegChildren(node, terms, seq.Order, seq.Negate)
	if err != nil {
		return nil, err
	}

	// Apply sequence-specific node properties
	if err := seqNodeProps(node, seq, seq.Order != nil, seq.Negate != nil); err != nil {
		return nil, err
	}

	node.Children = append(node.Children, pos...)
	node.Children = append(node.Children, neg...)
	if len(neg) > 0 {
		node.NegIdx = len(pos)
	}
	return node, nil
}

func buildSetNode(parent *NodeT, terms map[string]ParseTermT, set *ParseSetT) (*NodeT, error) {
	node := initNode(parent.Metadata.RuleId, parent.Metadata.RuleHash)

	pos, neg, err := buildPosNegChildren(node, terms, set.Match, set.Negate)
	if err != nil {
		return nil, err
	}

	// Apply set-specific node properties
	if err := setNodeProps(node, set, set.Match != nil, set.Negate != nil); err != nil {
		return nil, err
	}

	node.Children = append(node.Children, pos...)
	node.Children = append(node.Children, neg...)
	if len(neg) > 0 {
		node.NegIdx = len(pos)
	}
	return node, nil
}

// buildPosNegChildren is a helper for building
// positive and negative children across Sequence and Set
func buildPosNegChildren(node *NodeT, terms map[string]ParseTermT, matches, negates []ParseTermT) (pos []any, neg []any, err error) {

	pos, neg = []any{}, []any{}

	if len(matches) > 0 {
		cPos, err := buildChildren(node, terms, matches, false)
		if err != nil {
			return nil, nil, err
		}
		pos = append(pos, cPos...)
	}

	if len(negates) > 0 {
		cNeg, err := buildChildren(node, terms, negates, true)
		if err != nil {
			return nil, nil, err
		}
		neg = append(neg, cNeg...)
	}

	return pos, neg, nil
}

func parseValue(term ParseTermT, negate bool) (*MatcherT, error) {

	var (
		matcher = &MatcherT{}
	)

	switch negate {
	case false:
		matcher.Match.Fields = append(matcher.Match.Fields, FieldT{
			Field:      term.Field,
			StrValue:   term.StrValue,
			JqValue:    term.JqValue,
			RegexValue: term.RegexValue,
			Count:      term.Count,
		})
	case true:

		var (
			err  error
			opts *NegateOptsT
		)

		if term.NegateOpts != nil {
			if opts, err = negateOpts(term); err != nil {
				return nil, err
			}
		}

		matcher.Negate.Fields = append(matcher.Negate.Fields, FieldT{
			Field:      term.Field,
			StrValue:   term.StrValue,
			JqValue:    term.JqValue,
			RegexValue: term.RegexValue,
			Count:      term.Count,
			NegateOpts: opts,
		})
	}

	return matcher, nil
}

func ParseCres(data []byte) (map[string]ParseCreT, error) {
	var (
		config RulesT
		cres   = make(map[string]ParseCreT)
		err    error
	)

	if config, err = _parse(data); err != nil {
		return nil, err
	}

	for _, rule := range config.Rules {
		cres[rule.Metadata.Hash] = rule.Cre
	}

	return cres, nil
}

func Parse(data []byte) (*TreeT, error) {

	var (
		config RulesT
		err    error
	)

	if config, err = _parse(data); err != nil {
		return nil, err
	}

	return parseRules(config.Rules, config.Terms)
}

func parseRules(rules []ParseRuleT, terms map[string]ParseTermT) (*TreeT, error) {

	var (
		tree = &TreeT{
			Nodes: make([]*NodeT, 0),
		}
	)

	for _, rule := range rules {
		var (
			node *NodeT
			err  error
		)

		if node, err = buildTree(terms, rule); err != nil {
			return nil, err
		}

		tree.Nodes = append(tree.Nodes, node)
	}

	return tree, nil
}

func ParseRules(config *RulesT) (*TreeT, error) {
	return parseRules(config.Rules, config.Terms)
}
