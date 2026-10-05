package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"evonardy/internal/encoder"
	"evonardy/internal/game"
	"evonardy/internal/neural"
)

// NeuralParameters preserves value semantics in frozen policies and checkpoints.
type NeuralParameters [neural.ParameterCount]float64

func (p *NeuralParameters) UnmarshalJSON(data []byte) error {
	var values []float64
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	if _, err := neural.New(values); err != nil {
		return err
	}
	copy(p[:], values)
	return nil
}

// Neural owns a frozen network and has no training or exploration state.
type Neural struct {
	network neural.Network
}

func NewNeural(parameters []float64) (Neural, error) {
	n, err := neural.New(parameters)
	return Neural{network: n}, err
}

// Value is always from White's perspective; terminal wins are exact rewards.
func (n Neural) Value(p game.Position) (float64, error) {
	value, _, err := n.value(p)
	return value, err
}

func (n *Neural) value(p game.Position) (float64, bool, error) {
	input, err := encoder.Encode(p)
	if err != nil {
		return 0, false, err
	}
	if result, terminal := game.Result(p); terminal {
		if result.Winner == game.White {
			return 1, false, nil
		}
		return -1, false, nil
	}
	value, err := n.network.Forward(input)
	return value, true, err
}

func (n Neural) Choose(ctx context.Context, p game.Position, dice game.Dice, actions []game.Action) (int, error) {
	index, _, err := n.ChooseMeasured(ctx, p, dice, actions)
	return index, err
}

// ChooseMeasured consumes Go-generated legal successors, with stable first ties.
func (n Neural) ChooseMeasured(ctx context.Context, p game.Position, _ game.Dice, actions []game.Action) (int, int, error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	if len(actions) == 0 {
		return 0, 0, fmt.Errorf("neural agent needs legal actions")
	}
	if err := game.ValidatePosition(p); err != nil {
		return 0, 0, err
	}
	if index, ok := winning(p, actions); ok {
		return index, 0, nil
	}
	best, score, evaluations := 0, math.Inf(-1), 0
	for i, action := range actions {
		if err := ctx.Err(); err != nil {
			return 0, evaluations, err
		}
		value, evaluated, err := n.value(action.Next)
		if evaluated {
			evaluations++
		}
		if err != nil {
			return 0, evaluations, err
		}
		if p.Turn == game.Black {
			value = -value
		}
		if value > score {
			best, score = i, value
		}
	}
	return best, evaluations, nil
}
