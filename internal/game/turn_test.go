package game_test

import (
	"reflect"
	"testing"

	"evonardy/internal/game"
)

func TestFullTurnsDeduplicateButKeepHumanPaths(t *testing.T) {
	p := game.Initial(game.White)
	before := p
	dice := game.Dice{1, 2}
	paths, err := game.LegalPaths(p, dice)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatalf("want both orders, got %+v", paths)
	}
	turns, err := game.LegalTurns(p, dice)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 {
		t.Fatalf("one distinct final position, got %d", len(turns))
	}
	next, err := game.ApplyTurn(p, dice, turns[0])
	if err != nil {
		t.Fatal(err)
	}
	if next.Checkers[0][0] != 14 || next.Checkers[0][3] != 1 || next.Turn != game.Black || !next.FirstDone[0] {
		t.Fatalf("wrong full-turn result: %+v", next)
	}
	if p != before {
		t.Fatal("input mutated")
	}
	for _, invalid := range []game.Turn{{}, {Steps: paths[0].Steps[:1]}, {Steps: []game.Step{{From: 0, To: 3, Die: 3}}}} {
		if _, err := game.ApplyTurn(p, dice, invalid); err == nil {
			t.Fatalf("accepted %+v", invalid)
		}
	}
	options, err := game.LegalContinuations(p, dice, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Next) != 2 || options.Complete {
		t.Fatalf("wrong continuations: %+v", options)
	}
	options, err = game.LegalContinuations(p, dice, paths[0].Steps)
	if err != nil || !options.Complete || len(options.Next) != 0 {
		t.Fatalf("full path: %+v, %v", options, err)
	}
	if !reflect.DeepEqual(paths[0], turns[0]) {
		t.Fatal("representative must be stable")
	}
}
