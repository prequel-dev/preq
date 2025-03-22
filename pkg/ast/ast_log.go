package ast

import (
	"errors"
	"fmt"
	"time"

	"github.com/prequel-dev/detection-engine/pkg/parser"
	"github.com/prequel-dev/detection-engine/pkg/schema"
	"github.com/prequel-dev/prequel-logmatch/pkg/match"
	"github.com/rs/zerolog/log"
)

var (
	ErrUnknownField                  = errors.New("unknown source field")
	ErrUnknownSrc                    = errors.New("unknown source")
	ErrSeqPosConditions              = errors.New("sequences require two or more positive conditions")
	ErrMissingScalar                 = errors.New("missing string, jq, or regex condition")
	ErrMissingPositiveOrderCondition = errors.New("missing one or more positive condition under an order statement")
	ErrMissingPositiveMatchCondition = errors.New("missing one or more positive condition under a match statement")
)

type AstLogMatcherT struct {
	Event  AstEventT
	Match  []AstFieldT
	Negate []AstFieldT
	Window time.Duration
}

func validateLogSeq(n *parser.NodeT, t AstNodeTypeT, matches, negates int) error {

	if matches <= 1 {
		log.Error().
			Any("node", n).
			Str("type", t.String()).
			Msg("Window requires two or more positive conditions")
		return ErrSeqPosConditions
	}

	if n.Metadata.Window == 0 {
		log.Error().
			Any("node", n).
			Str("type", t.String()).
			Msg("Sequence requires a window")
		return ErrInvalidWindow
	}

	if matches == 0 {
		log.Error().
			Int("matches", matches).
			Int("negate", negates).
			Interface("node", n).
			Msg("Sequences require at least one order term")
		return ErrMissingPositiveOrderCondition
	}

	return nil
}

func validateLogSet(n *parser.NodeT, t AstNodeTypeT, matches, negates int) error {

	if matches > 1 && n.Metadata.Window == 0 {
		log.Error().
			Any("node", n).
			Str("type", t.String()).
			Msg("Window requires two or more positive conditions")
		return ErrInvalidWindow
	}

	if negates > 0 && matches == 0 {
		log.Error().
			Any("node", n).
			Str("type", t.String()).
			Msg("Sets require one or more positive conditions under a match statement")
		return ErrMissingPositiveMatchCondition
	}
	return nil
}

func buildLog(n *parser.NodeT, t AstNodeTypeT, depth int, parentMatchId uint32, matchId uint32) (*AstNodePairT, error) {

	var (
		matchFields  = make([]AstFieldT, 0)
		negateFields = make([]AstFieldT, 0)
		err          error
	)

	// Iterate over children scalars to validate
	for _, child := range n.Children {
		var (
			match *parser.MatcherT
			term  AstFieldT
			src   = n.Metadata.Event.Source
			ok    bool
		)

		// Children are expected to be scalar matcher values
		if match, ok = child.(*parser.MatcherT); !ok {
			log.Error().Interface("node", n).Int("depth", depth).Msg("Log set requires literal condition")
			return nil, ErrMissingScalar
		}

		// Count match fields and remember values
		for _, field := range match.Match.Fields {
			if field.Count > 1 {
				for i := 0; i < field.Count; i++ {
					if term, err = newMatchTerm(src, field); err != nil {
						log.Error().Err(err).Interface("node", n).Int("depth", depth).Msg("Invalid match field term")
						return nil, err
					}
					matchFields = append(matchFields, term)
				}
			} else {
				if term, err = newMatchTerm(src, field); err != nil {
					log.Error().Err(err).Interface("node", n).Int("depth", depth).Msg("Invalid match field term")
					return nil, err
				}
				matchFields = append(matchFields, term)
			}
		}

		// Count negate fields and remember values
		for _, field := range match.Negate.Fields {
			if field.Count > 1 {
				for range field.Count {
					if term, err = newNegateTerm(src, field); err != nil {
						log.Error().Err(err).Interface("node", n).Int("depth", depth).Msg("Invalid negate field term")
						return nil, err
					}
					negateFields = append(negateFields, term)
				}
			} else {
				if term, err = newNegateTerm(src, field); err != nil {
					log.Error().Err(err).Interface("node", n).Int("depth", depth).Msg("Invalid negate field term")
					return nil, err
				}
				negateFields = append(negateFields, term)
			}
		}
	}

	switch t {
	case NodeTypeLogSet:
		if err = validateLogSet(n, t, len(matchFields), len(negateFields)); err != nil {
			return nil, err
		}
	case NodeTypeLogSeq:
		if err = validateLogSeq(n, t, len(matchFields), len(negateFields)); err != nil {
			return nil, err
		}
	}

	return buildLogNodes(t, n, depth, parentMatchId, matchId, matchFields, negateFields)
}

func knownSrcField(src string, field parser.FieldT) (AstFieldT, error) {
	var (
		t = AstFieldT{
			Field: field.Field,
		}
		f, v string
	)

	switch src {
	case schema.EventTypeK8s:
		switch field.Field {
		case schema.FieldK8sEventReason:
			f = schema.FieldK8sEventReason
			v = field.StrValue
		case schema.FieldK8sEventType:
			f = schema.FieldK8sEventType
			v = field.StrValue
		case schema.FieldK8sEventReasonDetail:
			f = schema.FieldK8sEventReasonDetail
			v = field.StrValue
		default:
			return AstFieldT{}, ErrUnknownField
		}
	default:
		return AstFieldT{}, ErrUnknownSrc
	}

	t.TermValue = match.TermT{
		Type:  match.TermJqJson,
		Value: fmt.Sprintf("select(.%s == \"%s\")", f, v),
	}

	return t, nil
}

func newMatchTerm(src string, field parser.FieldT) (AstFieldT, error) {
	var (
		t     AstFieldT
		count = 0
		err   error
	)

	if t, err = knownSrcField(src, field); err == nil {
		return t, nil
	}

	t = AstFieldT{
		Field: field.Field,
	}

	if field.StrValue != "" {
		t.TermValue = match.TermT{
			Type:  match.TermRaw,
			Value: field.StrValue,
		}
		count++
	}
	if field.JqValue != "" {
		t.TermValue = match.TermT{
			Type:  match.TermJqJson,
			Value: field.JqValue,
		}
		count++
	}
	if field.RegexValue != "" {
		t.TermValue = match.TermT{
			Type:  match.TermRegex,
			Value: field.RegexValue,
		}
		count++
	}

	if count > 1 {
		log.Error().Msg("Only one of str, json, or regex value can be set")
		return AstFieldT{}, ErrInvalidNodeType
	}

	return t, nil
}

func newNegateTerm(src string, field parser.FieldT) (AstFieldT, error) {

	var (
		t   AstFieldT
		err error
	)

	if t, err = newMatchTerm(src, field); err != nil {
		return AstFieldT{}, err
	}

	if field.NegateOpts != nil {
		t.NegateOpts = &AstNegateOptsT{
			Window:   field.NegateOpts.Window,
			Slide:    field.NegateOpts.Slide,
			Anchor:   field.NegateOpts.Anchor,
			Absolute: field.NegateOpts.Absolute,
		}
	}

	return t, nil
}

func buildLogNodes(t AstNodeTypeT, n *parser.NodeT, depth int, parentMatchId uint32, matchId uint32, matchFields []AstFieldT, negateFields []AstFieldT) (*AstNodePairT, error) {
	var (
		scope      string
		matchNode  *AstNodeT
		assertNode = newAstNode(n, NodeTypeDesc, schema.ScopeCluster, depth, parentMatchId, matchId)
	)

	// TODO: revisit after data source abstraction
	if n.Metadata.Event.Source == schema.EventTypeK8s {
		scope = schema.ScopeCluster
	} else {
		scope = schema.ScopeNode
	}

	matchNode = newAstNode(n, t, scope, depth, parentMatchId, matchId)

	matchNode.Object = &AstLogMatcherT{
		Event: AstEventT{
			Origin: n.Metadata.Event.Origin,
			Source: n.Metadata.Event.Source,
		},
		Match:  matchFields,
		Negate: negateFields,
		Window: n.Metadata.Window,
	}

	assertNode.Object = &AstDescriptorT{
		Type:    NodeTypeLogSet,
		MatchId: matchId,
		Depth:   depth,
	}

	return &AstNodePairT{
		Match:      matchNode,
		Descriptor: assertNode,
	}, nil
}
