package ast

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/prequel-dev/detection-engine/pkg/parser"
	"github.com/prequel-dev/detection-engine/pkg/schema"
	"github.com/prequel-dev/prequel-logmatch/pkg/match"
	"github.com/rs/zerolog/log"
)

var (
	ErrInvalidEventType        = errors.New("invalid event type")
	ErrInvalidNodeType         = errors.New("invalid node type")
	ErrRootNodeWithoutEventSrc = errors.New("root node has no event src")
	ErrInvalidWindow           = errors.New("invalid window")
	ErrMissingOrigin           = errors.New("missing origin event")
	ErrInvalidAnchor           = errors.New("invalid anchor")
)

type AstNodeTypeT string
type AstMatchIdT uint

const (
	NodeTypeUnk    AstNodeTypeT = "Unknown"
	NodeTypeSeq    AstNodeTypeT = "machine_seq"
	NodeTypeSet    AstNodeTypeT = "machine_set"
	NodeTypeLogSeq AstNodeTypeT = "log_seq"
	NodeTypeLogSet AstNodeTypeT = "log_set"
	NodeTypeDesc   AstNodeTypeT = "desc"
)

func (t AstNodeTypeT) String() string {
	return string(t)
}

type AstT struct {
	Nodes []*AstNodeT
}

type AstFieldT struct {
	Field      string
	StrValue   string
	JsonValue  string
	RegexValue string
	TermValue  match.TermT
	NegateOpts *AstNegateOptsT
}

type AstEventT struct {
	Origin bool   `json:"origin"`
	Source string `json:"source"`
}

type AstMetadataT struct {
	Scope         string
	Type          AstNodeTypeT
	RuleId        string
	RuleHash      string
	MatchId       uint32
	ParentMatchId uint32
	Depth         int
	NegateOpts    *AstNegateOptsT
}

type AstNodeT struct {
	Metadata AstMetadataT
	Object   any
	Children []*AstNodeT
	NegIdx   int
}

type AstNegateOptsT struct {
	Window   time.Duration
	Slide    time.Duration
	Anchor   uint32
	Absolute bool
}

type AstDescriptorT struct {
	Type       AstNodeTypeT
	MatchId    uint32
	Depth      int
	NegateOpts *AstNegateOptsT
}

// Each matcher node requires a corresponding descriptor node (except for the root, which is a detection)
type AstNodePairT struct {
	Match      *AstNodeT
	Descriptor *AstNodeT
}

func isRootMatcher(node *parser.NodeT) bool {

	var (
		hasMatcher = true
	)

	for _, child := range node.Children {
		if _, ok := child.(*parser.MatcherT); !ok {
			hasMatcher = false
		}
	}

	return hasMatcher
}

// buildTreeForRootMatcher handles the logic for a node that is a root matcher.
func buildTreeForRootMatcher(node *parser.NodeT, astNode *AstNodeT, depth int, parentMatchId, matchId uint32, hasOrigin *bool) error {

	var (
		np  *AstNodePairT
		err error
	)

	// Root matcher nodes must be at depth 0.
	if depth != 0 {
		log.Error().
			Interface("node", node).
			Int("depth", depth).
			Msg("Root matcher node at depth != 0")
		return ErrInvalidNodeType
	}

	// Optional event at the root node
	if node.Metadata.Event != nil {
		*hasOrigin = true
		node.Metadata.Event.Origin = true

		if node.Metadata.Event.Source == "" {
			log.Error().
				Interface("node", node).
				Int("depth", depth+1).
				Msg("Event missing src")
			return ErrInvalidEventType
		}

		if np, err = buildMatcherNodes(node, depth+1, parentMatchId, matchId); err != nil {
			return err
		}

		astNode.Children = append(astNode.Children, np.Match, np.Descriptor)

		return nil
	}

	return ErrRootNodeWithoutEventSrc
}

// buildTreeForChildren handles the logic for a node that is not a root matcher,
// i.e., it processes child nodes recursively.
func buildTreeForChildren(node *parser.NodeT, astNode *AstNodeT, depth int, matchId uint32, hasOrigin *bool) error {

	var negateOpts *parser.NegateOptsT

	for i, child := range node.Children {

		var (
			mid       = uint32(i)
			childNode *parser.NodeT
			ok        bool
		)

		if childNode, ok = child.(*parser.NodeT); !ok {
			log.Error().
				Interface("child", child).
				Int("depth", depth+1).
				Msg("Invalid child type")
			return ErrInvalidNodeType
		}

		if childNode.Metadata.NegateOpts != nil {
			negateOpts = childNode.Metadata.NegateOpts

			if negateOpts.Anchor > uint32(len(node.Children)) {
				log.Error().
					Interface("node", node).
					Int("depth", depth+1).
					Msg("Negate anchor is greater than the number of children")
				return ErrInvalidAnchor
			}
		}

		// If the child has an event, build it via buildMatcherNodes
		if childNode.Metadata.Event != nil {

			var (
				np  *AstNodePairT
				err error
			)

			if childNode.Metadata.Event.Origin {
				*hasOrigin = true
			}

			if childNode.Metadata.Event.Source == "" {
				log.Error().
					Interface("node", childNode).
					Int("depth", depth+1).
					Msg("Event missing src")
				return ErrInvalidEventType
			}

			if np, err = buildMatcherNodes(childNode, depth+1, matchId, mid); err != nil {
				return err
			}

			addNegateOpts(np.Descriptor, negateOpts)

			astNode.Children = append(astNode.Children, np.Match, np.Descriptor)

		} else {

			var (
				newNp *AstNodePairT
				err   error
			)

			// Otherwise, recurse
			if newNp, err = buildTree(childNode, depth+1, matchId, mid, hasOrigin); err != nil {
				return err
			}

			addNegateOpts(newNp.Descriptor, negateOpts)

			astNode.Children = append(astNode.Children, newNp.Match, newNp.Descriptor)
			astNode.Metadata.MatchId = mid
			astNode.Metadata.Depth = newNp.Match.Metadata.Depth
		}
	}

	return nil
}

func addNegateOpts(desc *AstNodeT, negateOpts *parser.NegateOptsT) {
	if negateOpts == nil {
		return
	}

	if desc, ok := desc.Object.(*AstDescriptorT); ok {
		desc.NegateOpts = &AstNegateOptsT{
			Window:   negateOpts.Window,
			Slide:    negateOpts.Slide,
			Anchor:   negateOpts.Anchor,
			Absolute: negateOpts.Absolute,
		}
	}
}

// buildTree constructs the AST from the given parser node.
func buildTree(node *parser.NodeT, depth int, parentMatchId, matchId uint32, hasOrigin *bool) (*AstNodePairT, error) {

	var (
		astNode *AstNodeT
		np      *AstNodePairT
		err     error
	)

	// If the node is nil, return immediately
	if node == nil {
		return nil, nil
	}

	// Determine the node type for the AST
	var nodeType AstNodeTypeT
	switch node.Metadata.Type {
	case parser.NodeTypeSeq, parser.NodeTypeSeqNegSingle, parser.NodeTypeSeqNeg:
		nodeType = NodeTypeLogSeq
	case parser.NodeTypeSet, parser.NodeTypeSetNegSingle, parser.NodeTypeSetNeg:
		nodeType = NodeTypeLogSet
	default:
		log.Error().
			Interface("node", node).
			Int("depth", depth).
			Msg("Invalid node type")
		return nil, ErrInvalidNodeType
	}

	// Construct the AST node
	astNode = &AstNodeT{
		Metadata: AstMetadataT{
			RuleId:        node.Metadata.RuleId,
			RuleHash:      node.Metadata.RuleHash,
			Type:          nodeType,
			Scope:         schema.ScopeCluster,
			Depth:         depth,
			ParentMatchId: parentMatchId,
		},
		NegIdx: node.NegIdx,
	}

	// Delegate to helper functions based on whether it's a root matcher
	if isRootMatcher(node) {
		if err = buildTreeForRootMatcher(node, astNode, depth, parentMatchId, matchId, hasOrigin); err != nil {
			return nil, err
		}
	} else {
		if err = buildTreeForChildren(node, astNode, depth, matchId, hasOrigin); err != nil {
			return nil, err
		}
	}

	// Construct the final node pair via state machine
	if np, err = buildStateMachine(node, astNode.Children, depth, parentMatchId, matchId); err != nil {
		return nil, err
	}

	// Attach the children to the final match node
	np.Match.Children = append(np.Match.Children, astNode.Children...)

	return np, nil
}

func newAstNode(n *parser.NodeT, nodeType AstNodeTypeT, scope string, depth int, parentMatchId uint32, matchId uint32) *AstNodeT {
	return &AstNodeT{
		Metadata: AstMetadataT{
			RuleId:        n.Metadata.RuleId,
			RuleHash:      n.Metadata.RuleHash,
			MatchId:       matchId,
			ParentMatchId: parentMatchId,
			Type:          nodeType,
			Scope:         scope,
			Depth:         depth,
		},
		NegIdx: n.NegIdx,
	}
}

func buildMatcherNodes(n *parser.NodeT, depth int, parentMatchId uint32, matchId uint32) (*AstNodePairT, error) {

	switch n.Metadata.Type {
	case parser.NodeTypeSeq, parser.NodeTypeSeqNegSingle, parser.NodeTypeSeqNeg:
		return buildLog(n, NodeTypeLogSeq, depth, parentMatchId, matchId)

	case parser.NodeTypeSet, parser.NodeTypeSetNegSingle, parser.NodeTypeSetNeg:
		return buildLog(n, NodeTypeLogSet, depth, parentMatchId, matchId)

	default:
		log.Error().Interface("node", n).Int("depth", depth).Msg("Invalid node type")
		return nil, ErrInvalidNodeType
	}
}

func buildStateMachine(n *parser.NodeT, children []*AstNodeT, depth int, parentMatchId uint32, matchId uint32) (*AstNodePairT, error) {

	var (
		typ AstNodeTypeT
	)

	switch n.Metadata.Type {
	case parser.NodeTypeSeq, parser.NodeTypeSeqNeg:

		if n.Metadata.Window == 0 {
			log.Error().
				Any("node", children).
				Msg("Window is required for sequences")
			return nil, ErrInvalidWindow
		}
		typ = NodeTypeSeq
	case parser.NodeTypeSet, parser.NodeTypeSetNeg, parser.NodeTypeSetNegSingle:
		typ = NodeTypeSet

	default:
		log.Error().Interface("node", n).Msg("Invalid node type")
		return nil, ErrInvalidNodeType
	}

	return buildMachineNodes(n, children, depth, parentMatchId, matchId, typ)
}

func Build(data []byte) (*AstT, error) {

	var (
		parseTree *parser.TreeT
		err       error
	)

	if parseTree, err = parser.Parse(data); err != nil {
		return nil, err
	}

	return BuildTree(parseTree)
}

func BuildTree(tree *parser.TreeT) (*AstT, error) {
	var (
		ast = &AstT{
			Nodes: make([]*AstNodeT, 0),
		}
		hasOrigin          bool
		np                 *AstNodePairT
		startDepth         = 0
		startMatchId       = uint32(0)
		startParentMatchId = uint32(0)
		err                error
	)

	for _, rule := range tree.Nodes {

		if np, err = buildTree(rule, startDepth, startParentMatchId, startMatchId, &hasOrigin); err != nil {
			return nil, err
		}

		if !hasOrigin {
			log.Error().Interface("rule", rule).Msg("Rule has no origin event")
			return nil, ErrMissingOrigin
		}

		log.Debug().Interface("np", np).Msg("Appending nodes")
		ast.Nodes = append(ast.Nodes, np.Match)
		ast.Nodes = append(ast.Nodes, np.Descriptor)
	}

	return ast, nil
}

func traverseTree(node *AstNodeT, f *os.File, indent string) error {

	var (
		parent string
		child  string
		err    error
	)

	switch o := node.Object.(type) {
	case *AstSeqMatcherT:
		parent = fmt.Sprintf("%s.%d.%d w=%s pos_terms=%d neg_terms=%d scope=%s", node.Metadata.Type, node.Metadata.Depth, node.Metadata.MatchId, o.Window, len(o.Order), len(o.Negate), node.Metadata.Scope)
	case *AstSetMatcherT:
		parent = fmt.Sprintf("%s.%d.%d w=%s pos_terms=%d neg_terms=%d scope=%s", node.Metadata.Type, node.Metadata.Depth, node.Metadata.MatchId, o.Window, len(o.Match), len(o.Negate), node.Metadata.Scope)
	}

	if _, err = f.Write([]byte(fmt.Sprintf("%s%s\n", indent, parent))); err != nil {
		return err
	}

	indent += "  "

	for _, c := range node.Children {
		switch o := c.Object.(type) {
		case *AstLogMatcherT:
			child = fmt.Sprintf("%s.%d.%d w=%s pos_terms=%d neg_terms=%d scope=%s", c.Metadata.Type, c.Metadata.Depth, c.Metadata.MatchId, o.Window, len(o.Match), len(o.Negate), c.Metadata.Scope)
		case *AstDescriptorT:
			child = fmt.Sprintf("%s.%d.%d scope=%s", c.Metadata.Type, c.Metadata.Depth, c.Metadata.MatchId, c.Metadata.Scope)
		}

		if _, err = f.Write([]byte(fmt.Sprintf("%s%s\n", indent, child))); err != nil {
			return err
		}

		if err = traverseTree(c, f, indent+"  "); err != nil {
			return err
		}
	}

	return nil
}

func DrawTree(tree *AstT, path string) error {
	var (
		f   *os.File
		err error
	)

	if f, err = os.Create(path); err != nil {
		return err
	}

	for _, node := range tree.Nodes {
		if err = traverseTree(node, f, ""); err != nil {
			return err
		}
	}

	return nil
}
