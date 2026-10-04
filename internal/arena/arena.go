// Package arena runs bounded, reproducible baseline simulations.
package arena

import (
	"context"
	"evonardy/internal/agent"
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
	names := [2]string{cfg.White, cfg.Black}
	var policies [2]Factory
	for side, name := range names {
		policies[side] = Factory{ID: "builtin/" + name + "-v1", New: func(r agent.IntSource) agent.Agent {
			if name == "heuristic" {
				return agent.Heuristic{}
			}
			return agent.NewRandom(r)
		}}
	}
	result, err := Play(ctx, MatchConfig{Seed: cfg.Seed, ID: id, StreamID: id, MaxTurns: cfg.MaxTurns}, policies)
	return result.Record, err
}
