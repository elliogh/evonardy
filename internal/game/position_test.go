package game_test

import (
	"testing"

	"evonardy/internal/game"
)

func TestInitialPositionAndCoordinates(t *testing.T) {
	p := game.Initial(game.Black)
	if err := game.ValidatePosition(p); err != nil {
		t.Fatal(err)
	}
	if p.Checkers[game.White][0] != 15 || p.Checkers[game.Black][12] != 15 {
		t.Fatalf("heads: %+v", p.Checkers)
	}
	if game.Progress(game.Black, 0) != 12 || game.PhysicalPoint(game.Black, 18) != 6 {
		t.Fatal("black coordinates must cross physical zero")
	}
	if p.Turn != game.Black || p.Starter != game.Black {
		t.Fatal("starter must move first")
	}
}

func TestRejectMalformedPosition(t *testing.T) {
	for _, change := range []func(*game.Position){
		func(p *game.Position) { p.Checkers[0][0] = 14 },
		func(p *game.Position) { p.Checkers[0][0] = -1 },
		func(p *game.Position) { p.Checkers[1][0] = 1; p.Checkers[1][12] = 14 },
		func(p *game.Position) { p.Turn = game.Player(2) },
		func(p *game.Position) { p.Ruleset = "short-backgammon" },
	} {
		p := game.Initial(game.White)
		change(&p)
		if game.ValidatePosition(p) == nil {
			t.Fatalf("accepted invalid position: %+v", p)
		}
	}
}
