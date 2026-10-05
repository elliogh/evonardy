package agent_test

import (
	"context"
	"evonardy/internal/agent"
	"evonardy/internal/game"
	"evonardy/internal/random"
	"testing"
)

func TestBaselinesChooseLegalAndDeterministicActions(t *testing.T) {
	p := game.Initial(game.White)
	p.Checkers[0][0] = 13
	p.Checkers[0][2] = 1
	p.Checkers[0][5] = 1
	p.FirstDone = [2]bool{true, true}
	dice := game.Dice{1, 2}
	actions, err := game.LegalActions(p, dice)
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) < 2 {
		t.Fatal("fixture must have multiple distinct states")
	}
	for _, bot := range []agent.Agent{agent.NewRandom(random.New(8, "agent", 0)), agent.Heuristic{}} {
		index, err := bot.Choose(context.Background(), p, dice, actions)
		if err != nil || index < 0 || index >= len(actions) {
			t.Fatalf("illegal choice %d %v", index, err)
		}
	}
	a, b := agent.NewRandom(random.New(8, "agent", 0)), agent.NewRandom(random.New(8, "agent", 0))
	counts := make([]int, len(actions))
	for i := 0; i < 5000; i++ {
		x, err := a.Choose(context.Background(), p, dice, actions)
		if err != nil {
			t.Fatal(err)
		}
		y, err := b.Choose(context.Background(), p, dice, actions)
		if err != nil {
			t.Fatal(err)
		}
		if x != y {
			t.Fatal("random not reproducible")
		}
		counts[x]++
	}
	expected := 5000 / len(actions)
	for _, n := range counts {
		if n < expected*7/10 || n > expected*13/10 {
			t.Fatalf("nonuniform sample: %v", counts)
		}
	}
	h := agent.Heuristic{}
	x, _ := h.Choose(context.Background(), p, dice, actions)
	y, _ := h.Choose(context.Background(), p, dice, actions)
	if x != y {
		t.Fatal("heuristic not stable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Choose(ctx, p, dice, actions); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestImmediateVictoryOverridesHeuristicScore(t *testing.T) {
	p := game.Initial(game.White)
	p.Checkers[0] = [24]int{}
	p.Checkers[0][21] = 1
	p.BorneOff[0] = 14
	p.FirstDone = [2]bool{true, true}
	actions, err := game.LegalActions(p, game.Dice{1, 3})
	if err != nil {
		t.Fatal(err)
	}
	for _, bot := range []agent.Agent{agent.Heuristic{}, agent.NewRandom(random.New(5, "agent", 0))} {
		i, err := bot.Choose(context.Background(), p, game.Dice{1, 3}, actions)
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := game.Result(actions[i].Next); !ok {
			t.Fatal("missed immediate victory")
		}
	}
}
