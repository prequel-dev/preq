package compiler

import (
	"github.com/prequel-dev/detection-engine/pkg/ast"
	"github.com/rs/zerolog/log"
)

type NodePlugin struct{}

func NewNodePlugin() *NodePlugin {
	return &NodePlugin{}
}

func (p *NodePlugin) Compile(runtime RuntimeI, node *ast.AstNodeT, mid uint32) (ObjsT, error) {

	var (
		objs = make(ObjsT, 0)
		obj  *ObjT
		err  error
	)

	switch node.Metadata.Type {
	case ast.NodeTypeLogSeq, ast.NodeTypeLogSet:
		if obj, err = ObjLogMatcher(runtime, node, mid); err != nil {
			log.Error().Err(err).Str("scope", node.Metadata.Scope).Msg("Failed to compile matchers")
			return nil, err
		}
	default:
		log.Error().
			Interface("node_type", node.Metadata.Type).
			Interface("node", node).
			Msg("Unsupported node type")
		return nil, ErrUnsupportedNodeType
	}

	objs = append(objs, obj)

	return objs, nil
}
