// Package arena runs reproducible games with frozen, per-game policy instances.
package arena

import (
	"context"
	"evonardy/internal/agent"
	"evonardy/internal/game"
	"evonardy/internal/random"
	"evonardy/internal/replay"
	"fmt"
)

type Factory struct {
	ID  string
	New func(agent.IntSource) agent.Agent
}
type MatchConfig struct {
	Seed     uint64
	ID       uint64
	StreamID uint64
	MaxTurns int
}
type MatchResult struct {
	Record             replay.Record
	Decisions          int
	ForwardEvaluations int
}

func Play(ctx context.Context, cfg MatchConfig, factories [2]Factory) (MatchResult, error) {
	if cfg.MaxTurns < 1 || cfg.MaxTurns > replay.MaxEvents {
		return MatchResult{}, fmt.Errorf("invalid match turn limit")
	}
	id := cfg.ID
	report := MatchResult{}
	label := fmt.Sprintf("game/%d", cfg.StreamID)
	opening := []game.Dice{}
	starter := game.White
	for attempt := uint64(0); ; attempt++ {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		if attempt >= replay.MaxEvents {
			return report, fmt.Errorf("opening retry limit reached")
		}
		source := random.New(cfg.Seed, label+"/opening", attempt)
		dice := game.Dice{source.IntN(6) + 1, source.IntN(6) + 1}
		opening = append(opening, dice)
		if dice[0] != dice[1] {
			if dice[1] > dice[0] {
				starter = game.Black
			}
			break
		}
	}
	p := game.Initial(starter)
	var bots [2]agent.Agent
	var botIDs [2]string
	for side, factory := range factories {
		if factory.New == nil {
			return report, fmt.Errorf("missing frozen policy")
		}
		bots[side] = factory.New(random.New(cfg.Seed, fmt.Sprintf("%s/agent/%d", label, side), 0))
		botIDs[side] = factory.ID
		if bots[side] == nil {
			return report, fmt.Errorf("missing agent")
		}
	}
	r := replay.New(p, opening, botIDs, cfg.Seed, id, cfg.MaxTurns)
	report.Record = r
	var sideTurns [2]uint64
	for turnNumber := 0; turnNumber < cfg.MaxTurns; turnNumber++ {
		if err := ctx.Err(); err != nil {
			report.Record = r
			return report, err
		}
		player := p.Turn
		dice := opening[len(opening)-1]
		if turnNumber > 0 {
			source := random.New(cfg.Seed, fmt.Sprintf("%s/dice/%d", label, player), sideTurns[player])
			dice = game.Dice{source.IntN(6) + 1, source.IntN(6) + 1}
		}
		sideTurns[player]++
		actions, err := game.LegalActions(p, dice)
		if err != nil {
			report.Record = r
			return report, fmt.Errorf("game %d legality: %w", id, err)
		}
		var choice int
		if measured, ok := bots[player].(interface {
			ChooseMeasured(context.Context, game.Position, game.Dice, []game.Action) (int, int, error)
		}); ok {
			var evaluations int
			choice, evaluations, err = measured.ChooseMeasured(ctx, p, dice, actions)
			report.ForwardEvaluations += evaluations
		} else {
			choice, err = bots[player].Choose(ctx, p, dice, actions)
		}
		if err != nil {
			report.Record = r
			return report, err
		}
		if choice < 0 || choice >= len(actions) {
			report.Record = r
			return report, fmt.Errorf("agent returned invalid action index")
		}
		action := actions[choice]
		next, err := game.ApplyTurn(p, dice, action.Turn)
		if err != nil {
			report.Record = r
			return report, fmt.Errorf("game %d illegal agent action: %w", id, err)
		}
		if next != action.Next {
			report.Record = r
			return report, fmt.Errorf("successor/application mismatch")
		}
		report.Decisions++
		r.Append(dice, action.Turn, next)
		p = next
		if _, terminal := game.Result(p); terminal {
			r.Finish(p, replay.Completed)
			report.Record = r
			return report, nil
		}
	}
	r.Finish(p, replay.Truncated)
	report.Record = r
	return report, nil
}
