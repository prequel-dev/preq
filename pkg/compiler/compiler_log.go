package compiler

import (
	"errors"
	"fmt"

	"github.com/prequel-dev/detection-engine/pkg/ast"
	"github.com/prequel-dev/prequel-logmatch/pkg/match"
	"github.com/rs/zerolog/log"
)

const (
	SubjectLogMatch = "local.match.log.%s.%d.%d"
)

var (
	ErrUnsupportedNodeType  = errors.New("unsupported node type")
	ErrUnsupportedEventType = errors.New("unsupported event type")
	ErrSequenceSingleMatch  = errors.New("sequence with single match (use set instead)")
	ErrNoFields             = errors.New("no fields")
)

func GetLogMatchSubject(ruleId string, depth int, matcherId uint32) string {
	return fmt.Sprintf(SubjectLogMatch, ruleId, depth, matcherId)
}

func toLogResets(terms []ast.AstFieldT) []match.ResetT {
	resets := make([]match.ResetT, 0, len(terms))
	for _, term := range terms {

		if term.NegateOpts == nil {
			resets = append(resets, match.ResetT{
				Term: term.TermValue,
			})
			continue
		}

		resets = append(resets, match.ResetT{
			Term:     term.TermValue,
			Window:   term.NegateOpts.Window.Nanoseconds(),
			Slide:    term.NegateOpts.Slide.Nanoseconds(),
			Anchor:   uint8(term.NegateOpts.Anchor),
			Absolute: term.NegateOpts.Absolute,
		})

		log.Debug().Any("reset", resets[len(resets)-1]).Msg("Adding log resets")
	}
	return resets
}

func toLogTerms(fields []ast.AstFieldT) []match.TermT {
	terms := make([]match.TermT, 0, len(fields))
	for _, field := range fields {
		terms = append(terms, field.TermValue)
	}
	return terms
}

func ObjLogMatcher(runtime RuntimeI, node *ast.AstNodeT, mid uint32) (*ObjT, error) {
	var (
		obj = NewObj(node)
		lm  *ast.AstLogMatcherT
		ok  bool
		err error
	)

	if lm, ok = node.Object.(*ast.AstLogMatcherT); !ok {
		log.Error().Interface("matcher", node.Object).Msg("Failed to compile log matcher")
		return nil, ErrInvalidMatcher
	}

	obj.Event.Origin = lm.Event.Origin
	obj.Event.Source = lm.Event.Source

	params := MatchParamsT{
		RuleId:   node.Metadata.RuleId,
		RuleHash: node.Metadata.RuleHash,
		MatchId:  mid,
		Subject:  GetLogMatchSubject(node.Metadata.RuleHash, node.Metadata.Depth, mid),
		NodeType: node.Metadata.Type,
		Origin:   lm.Event.Origin,
	}

	obj.Cb = CbT{
		Callback: runtime.NewCbMatch(params),
	}

	switch node.Metadata.Type {
	case ast.NodeTypeLogSeq:
		if obj.Object, err = makeLogSeqObjects(lm, node.NegIdx); err != nil {
			return nil, err
		}

	case ast.NodeTypeLogSet:

		if obj.Object, err = makeLogSetObjects(lm, node.NegIdx); err != nil {
			return nil, err
		}

	default:
		log.Error().Type("node_type", node.Metadata.Type).Msg("Unsupported node type")
		return nil, ErrUnsupportedNodeType
	}

	return obj, nil
}

func makeLogSeqObjects(lm *ast.AstLogMatcherT, negIdx int) (any, error) {

	var (
		obj any
		err error
	)

	if negIdx > 0 {
		log.Debug().Any("terms", toLogTerms(lm.Match)).Msg("Creating inverse match sequence")
		if obj, err = match.NewInverseSeq(lm.Window.Nanoseconds(), toLogTerms(lm.Match), toLogResets(lm.Negate)); err != nil {
			log.Error().Err(err).Msg("Failed to create inverse match sequence")
			return nil, err
		}
	} else {
		if len(lm.Match) == 1 {
			log.Error().Msg("Sequence with single match (use set instead)")
			return nil, ErrSequenceSingleMatch
		} else {
			log.Debug().Any("terms", toLogTerms(lm.Match)).Msg("Creating match sequence")
			if obj, err = match.NewMatchSeq(lm.Window.Nanoseconds(), toLogTerms(lm.Match)...); err != nil {
				log.Error().Err(err).Msg("Failed to create match sequence")
				return nil, err
			}
		}
	}

	return obj, nil
}

func makeLogSetObjects(lm *ast.AstLogMatcherT, negIdx int) (any, error) {

	var (
		err error
		obj any
	)

	if negIdx > 0 {
		log.Debug().Any("terms", toLogTerms(lm.Match)).Msg("Creating inverse match set")
		if obj, err = match.NewInverseSet(lm.Window.Nanoseconds(), toLogTerms(lm.Match), toLogResets(lm.Negate)); err != nil {
			log.Error().Err(err).Msg("Failed to create inverse match set")
			return nil, err
		}
	} else {
		if len(lm.Match) == 1 {
			log.Debug().Any("term", toLogTerms(lm.Match)[0]).Msg("Creating match single")
			if obj, err = match.NewMatchSingle(toLogTerms(lm.Match)[0]); err != nil {
				log.Error().Err(err).Msg("Failed to create match single")
				return nil, err
			}
		} else {
			log.Debug().Any("terms", toLogTerms(lm.Match)).Msg("Creating match set")
			if obj, err = match.NewMatchSet(lm.Window.Nanoseconds(), toLogTerms(lm.Match)...); err != nil {
				log.Error().Err(err).Msg("Failed to create match set")
				return nil, err
			}
		}
	}

	return obj, nil
}
