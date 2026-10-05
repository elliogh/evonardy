// Package agent contains inference-only baselines. Agents choose full actions.
package agent

import (
	"context"
	"evonardy/internal/features"
	"evonardy/internal/game"
	"fmt"
	"math"
)

type Agent interface {
	Choose(context.Context, game.Position, game.Dice, []game.Action) (int, error)
}

type IntSource interface{ IntN(int) int }
type Random struct{ source IntSource }

func NewRandom(source IntSource) *Random { return &Random{source: source} }

func (r *Random) Choose(ctx context.Context, p game.Position, _ game.Dice, actions []game.Action) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(actions) == 0 || r.source == nil {
		return 0, fmt.Errorf("random agent needs legal actions and a random source")
	}
	if index, ok := winning(p, actions); ok {
		return index, nil
	}
	return r.source.IntN(len(actions)), nil
}

type Heuristic struct{}

func (Heuristic) Choose(ctx context.Context, p game.Position, dice game.Dice, actions []game.Action) (int, error) {
	return (Linear{Weights: features.DefaultWeights}).Choose(ctx, p, dice, actions)
}

// Linear holds a value copy of frozen weights and never runs training.
type Linear struct{ Weights features.Vector }

func (l Linear) Choose(ctx context.Context, p game.Position, _ game.Dice, actions []game.Action) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if len(actions) == 0 {
		return 0, fmt.Errorf("heuristic agent needs legal actions")
	}
	if index, ok := winning(p, actions); ok {
		return index, nil
	}
	best, score := 0, math.Inf(-1)
	for i, action := range actions {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		value := features.Score(action.Next, l.Weights)
		if p.Turn == game.Black {
			value = -value
		}
		if value > score {
			best, score = i, value
		}
	}
	return best, nil
}

func winning(p game.Position, actions []game.Action) (int, bool) {
	for i, action := range actions {
		if result, terminal := game.Result(action.Next); terminal && result.Winner == p.Turn {
			return i, true
		}
	}
	return 0, false
}
