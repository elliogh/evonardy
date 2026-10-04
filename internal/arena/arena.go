// Package arena runs bounded, reproducible baseline simulations.
package arena

import (
	"context"
	"evonardy/internal/agent"
	"evonardy/internal/game"
	"evonardy/internal/random"
	"evonardy/internal/replay"
	"fmt"
	"sync"
	"sync/atomic"
)

type Config struct {
	Seed     uint64 `json:"seed"`
	Games    int    `json:"games"`
	Workers  int    `json:"workers"`
	MaxTurns int    `json:"max_turns"`
	White    string `json:"white"`
	Black    string `json:"black"`
}

func (c Config) Validate() error {
	if c.Games < 1 || c.Games > 1000 || c.Workers < 1 || c.Workers > 64 || c.MaxTurns < 1 || c.MaxTurns > replay.MaxEvents {
		return fmt.Errorf("games must be 1..1000, workers 1..64, max_turns 1..%d", replay.MaxEvents)
	}
	if c.Games*c.MaxTurns > 1000000 {
		return fmt.Errorf("batch budget exceeds 1000000 turn slots; split into smaller batches")
	}
	for _, name := range []string{c.White, c.Black} {
		if name != "random" && name != "heuristic" {
			return fmt.Errorf("unknown baseline %q", name)
		}
	}
	return nil
}

func Run(ctx context.Context, cfg Config) ([]replay.Record, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	results := make([]replay.Record, cfg.Games)
	var next atomic.Int64
	var wg sync.WaitGroup
	var once sync.Once
	var failure error
	for worker := 0; worker < min(cfg.Workers, cfg.Games); worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				id := int(next.Add(1) - 1)
				if id >= cfg.Games {
					return
				}
				r, err := simulate(ctx, cfg, uint64(id))
				if err != nil {
					once.Do(func() { failure = err; cancel() })
					return
				}
				results[id] = r
			}
		}()
	}
	wg.Wait()
	if failure != nil {
		return nil, failure
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func simulate(ctx context.Context, cfg Config, id uint64) (replay.Record, error) {
	label := fmt.Sprintf("game/%d", id)
	opening := []game.Dice{}
	starter := game.White
	for attempt := uint64(0); ; attempt++ {
		if err := ctx.Err(); err != nil {
			return replay.Record{}, err
		}
		if attempt >= replay.MaxEvents {
			return replay.Record{}, fmt.Errorf("opening retry limit reached")
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
	names := [2]string{cfg.White, cfg.Black}
	var bots [2]agent.Agent
	for player := game.White; player <= game.Black; player++ {
		if names[player] == "heuristic" {
			bots[player] = agent.Heuristic{}
		} else {
			bots[player] = agent.NewRandom(random.New(cfg.Seed, fmt.Sprintf("%s/agent/%d", label, player), 0))
		}
	}
	botIDs := [2]string{"builtin/" + names[0] + "-v1", "builtin/" + names[1] + "-v1"}
	r := replay.New(p, opening, botIDs, cfg.Seed, id, cfg.MaxTurns)
	var sideTurns [2]uint64
	for turnNumber := 0; turnNumber < cfg.MaxTurns; turnNumber++ {
		if err := ctx.Err(); err != nil {
			return r, err
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
			return r, fmt.Errorf("game %d legality: %w", id, err)
		}
		choice, err := bots[player].Choose(ctx, p, dice, actions)
		if err != nil {
			return r, err
		}
		if choice < 0 || choice >= len(actions) {
			return r, fmt.Errorf("agent returned invalid action index")
		}
		action := actions[choice]
		next, err := game.ApplyTurn(p, dice, action.Turn)
		if err != nil {
			return r, fmt.Errorf("game %d illegal agent action: %w", id, err)
		}
		if next != action.Next {
			return r, fmt.Errorf("successor/application mismatch")
		}
		r.Append(dice, action.Turn, next)
		p = next
		if _, terminal := game.Result(p); terminal {
			r.Finish(p, replay.Completed)
			return r, nil
		}
	}
	r.Finish(p, replay.Truncated)
	return r, nil
}
