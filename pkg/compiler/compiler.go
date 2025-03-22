package compiler

import (
	"errors"
	"sort"

	"github.com/prequel-dev/detection-engine/pkg/ast"
	"github.com/prequel-dev/detection-engine/pkg/parser"
	"github.com/rs/zerolog/log"
)

var (
	ErrUnsupportedMatcher = errors.New("unsupported matcher")
	ErrUnsupportedScope   = errors.New("unsupported scope")
	ErrInvalidMatcher     = errors.New("invalid matcher")
)

var (
	defaultPlugin  = &NodePlugin{}
	defaultRuntime = &NoopRuntime{}
)

type CbType uint
type CorrelationsT map[string]string

type CbT struct {
	Callback any
}

type ObjsT []*ObjT

type ObjT struct {
	RuleId        string           `json:"rule_id"`
	RuleHash      string           `json:"rule_hash"`
	MatchId       uint32           `json:"match_id"`
	ParentMatchId uint32           `json:"parent_match_id"`
	Depth         int              `json:"depth"`
	Scope         string           `json:"scope"`
	Type          ast.AstNodeTypeT `json:"type"`
	Event         ast.AstEventT    `json:"event"`
	Object        any              `json:"object"`
	Cb            CbT              `json:"cb"`
}

type compilerOptsT struct {
	debugTree string
	runtime   RuntimeI
	plugins   map[string]PluginI
}

type CompilerOptT func(*compilerOptsT)
type PluginI interface {
	Compile(runtime RuntimeI, node *ast.AstNodeT, mid uint32) (ObjsT, error)
}

func WithDebugTree(path string) CompilerOptT {
	return func(o *compilerOptsT) {
		o.debugTree = path
	}
}

func WithRuntime(cb RuntimeI) CompilerOptT {
	return func(o *compilerOptsT) {
		o.runtime = cb
	}
}

func WithPlugin(scope string, plugin PluginI) CompilerOptT {
	return func(o *compilerOptsT) {
		o.plugins[scope] = plugin
	}
}

func parseOpts(opts []CompilerOptT) compilerOptsT {
	o := compilerOptsT{
		plugins: map[string]PluginI{"node": defaultPlugin},
		runtime: defaultRuntime,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func traverseTree(node *ast.AstNodeT, scope string, callback func(node *ast.AstNodeT) error) error {
	for _, child := range node.Children {
		if err := traverseTree(child, scope, callback); err != nil {
			return err
		}
	}
	return callback(node)
}

func NewObj(node *ast.AstNodeT) *ObjT {
	return &ObjT{
		RuleId:        node.Metadata.RuleId,
		RuleHash:      node.Metadata.RuleHash,
		MatchId:       node.Metadata.MatchId,
		ParentMatchId: node.Metadata.ParentMatchId,
		Depth:         node.Metadata.Depth,
		Scope:         node.Metadata.Scope,
		Type:          node.Metadata.Type,
	}
}

func sortObjs(items []*ObjT, t ast.AstNodeTypeT) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Type == t && items[j].Type != t {
			return true
		}
		if items[j].Type == t && items[i].Type != t {
			return false
		}
		return false
	})
}

func CompileTree(pt *parser.TreeT, scope string, opts ...CompilerOptT) (ObjsT, error) {

	var (
		err  error
		o    = parseOpts(opts)
		tree *ast.AstT
	)

	if tree, err = ast.BuildTree(pt); err != nil {
		return nil, err
	}

	if o.debugTree != "" {
		if err = ast.DrawTree(tree, o.debugTree); err != nil {
			return nil, err
		}
	}

	return compile(o, tree, scope)
}

func compile(o compilerOptsT, tree *ast.AstT, scope string) (ObjsT, error) {

	var (
		err     error
		outObjs ObjsT
	)

	compile := func(node *ast.AstNodeT) error {

		if node.Metadata.Scope != scope {
			return nil
		}

		plugin, ok := o.plugins[scope]
		if !ok {
			log.Error().Str("scope", scope).Msg("No plugin found")
			return ErrUnsupportedScope
		}

		objs, err := plugin.Compile(o.runtime, node, node.Metadata.MatchId)
		if err != nil {
			log.Error().
				Err(err).
				Str("scope", scope).
				Msg("Failed to compile")
			return err
		}

		outObjs = append(outObjs, objs...)

		return nil
	}

	for _, node := range tree.Nodes {
		if err = traverseTree(node, scope, compile); err != nil {
			return nil, err
		}
	}

	sortObjs(outObjs, ast.NodeTypeDesc)
	sortObjs(outObjs, ast.NodeTypeSeq)
	sortObjs(outObjs, ast.NodeTypeSet)

	for _, obj := range outObjs {
		log.Info().
			Str("obj.type", obj.Type.String()).
			Int("depth", obj.Depth).
			Int("match_id", int(obj.MatchId)).
			Msg("Compiled object")
	}

	return outObjs, nil
}

func Compile(data []byte, scope string, opts ...CompilerOptT) (ObjsT, error) {
	var (
		tree *ast.AstT
		o    = parseOpts(opts)
		err  error
	)

	if tree, err = ast.Build(data); err != nil {
		return nil, err
	}

	if o.debugTree != "" {
		if err = ast.DrawTree(tree, o.debugTree); err != nil {
			return nil, err
		}
	}

	return compile(o, tree, scope)
}
