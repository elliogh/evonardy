package arena_test

import (
	"context"
	"evonardy/internal/agent"
	"evonardy/internal/arena"
	"evonardy/internal/game"
	"evonardy/internal/random"
	"evonardy/internal/replay"
	"testing"
)

func TestFrozenPoliciesUsePairedEnvironmentStreamsAndMeasuredWork(t *testing.T) {
	factories := [2]arena.Factory{
		{ID: "builtin/heuristic-v1", New: func(agent.IntSource) agent.Agent { return agent.Heuristic{} }},
		{ID: "builtin/random-v1", New: func(r agent.IntSource) agent.Agent { return agent.NewRandom(r) }},
	}
	a, err := arena.Play(context.Background(), arena.MatchConfig{Seed: 42, ID: 0, StreamID: 0, MaxTurns: 1200}, factories)
	if err != nil {
		t.Fatal(err)
	}
	b, err := arena.Play(context.Background(), arena.MatchConfig{Seed: 42, ID: 1, StreamID: 0, MaxTurns: 1200}, [2]arena.Factory{factories[1], factories[0]})
	if err != nil {
		t.Fatal(err)
	}
	if a.Record.Status != replay.Completed || b.Record.Status != replay.Completed || a.Decisions != len(a.Record.Events) || a.ForwardEvaluations < 1 {
		t.Fatal("unmeasured or incomplete match")
	}
	for _, r := range []replay.Record{a.Record, b.Record} {
		if _, err := replay.Play(r); err != nil {
			t.Fatal(err)
		}
	}
	if a.Record.Initial != b.Record.Initial || a.Record.Opening[len(a.Record.Opening)-1] != b.Record.Opening[len(b.Record.Opening)-1] {
		t.Fatal("paired opening changed")
	}
	diceBySide := func(r replay.Record) [2][]game.Dice {
		var d [2][]game.Dice
		p := r.Initial.Turn
		for _, e := range r.Events {
			d[p] = append(d[p], e.Dice)
			p = p.Other()
		}
		return d
	}
	da, db := diceBySide(a.Record), diceBySide(b.Record)
	for p := game.White; p <= game.Black; p++ {
		for i := 0; i < min(len(da[p]), len(db[p])); i++ {
			if da[p][i] != db[p][i] {
				t.Fatal("dice depend on opponent decisions")
			}
		}
	}
	// The keyed seed source remains independent of policy calls.
	source := random.New(42, "test", 0)
	if source.Uint64() == source.Uint64() {
		t.Fatal("seed source did not advance")
	}
}
