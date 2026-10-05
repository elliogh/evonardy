package encoder_test

import (
	"encoding/json"
	"math"
	"testing"

	"evonardy/internal/encoder"
	"evonardy/internal/game"
)

func TestVersionedContract(t *testing.T) {
	if encoder.Version != "long-nardy-encoder-v1" || encoder.Size != 56 || encoder.ValuePerspective != "white" {
		t.Fatal("encoder v1 requires its fixed version, 56 inputs, and White value perspective")
	}
}

func TestExactPositionVectors(t *testing.T) {
	tests := []struct {
		name     string
		position game.Position
		want     encoder.Vector
	}{
		{
			name:     "White opening",
			position: game.Initial(game.White),
			want:     encoder.Vector{0: 1, 36: 1, 50: 1, 52: 1, 53: 1, 54: 1},
		},
		{
			name:     "Black opening keeps global points",
			position: game.Initial(game.Black),
			want:     encoder.Vector{0: 1, 36: 1, 51: 1, 52: 1, 53: 1, 55: 1},
		},
		{
			name: "asymmetric midgame",
			position: game.Position{
				Ruleset:  game.Ruleset,
				Checkers: [2][24]int{{0: 9, 11: 2, 23: 4}, {1: 1, 7: 3, 12: 9, 22: 2}},
				Turn:     game.Black, Starter: game.White, FirstDone: [2]bool{true, true},
			},
			want: encoder.Vector{0: 9.0 / 15, 11: 2.0 / 15, 23: 4.0 / 15, 25: 1.0 / 15, 31: 3.0 / 15, 36: 9.0 / 15, 46: 2.0 / 15, 51: 1, 54: 1},
		},
		{
			name: "Black across global zero and final point",
			position: game.Position{
				Ruleset:  game.Ruleset,
				Checkers: [2][24]int{{1: 9, 12: 2, 22: 4}, {0: 3, 6: 2, 11: 1, 23: 9}},
				Turn:     game.Black, Starter: game.Black, FirstDone: [2]bool{true, true},
			},
			want: encoder.Vector{1: 9.0 / 15, 12: 2.0 / 15, 22: 4.0 / 15, 24: 3.0 / 15, 30: 2.0 / 15, 35: 1.0 / 15, 47: 9.0 / 15, 51: 1, 55: 1},
		},
		{
			name: "both colors bearing off",
			position: game.Position{
				Ruleset:  game.Ruleset,
				Checkers: [2][24]int{{18: 2, 20: 3, 23: 4}, {6: 1, 9: 3, 11: 2}},
				BorneOff: [2]int{6, 9},
				Turn:     game.White, Starter: game.Black, FirstDone: [2]bool{true, true},
			},
			want: encoder.Vector{18: 2.0 / 15, 20: 3.0 / 15, 23: 4.0 / 15, 30: 1.0 / 15, 33: 3.0 / 15, 35: 2.0 / 15, 48: 6.0 / 15, 49: 9.0 / 15, 50: 1, 55: 1},
		},
		{
			name: "White terminal victory",
			position: game.Position{
				Ruleset:  game.Ruleset,
				Checkers: [2][24]int{{}, {6: 3, 8: 4, 11: 5}},
				BorneOff: [2]int{15, 3},
				Turn:     game.Black, Starter: game.White, FirstDone: [2]bool{true, true},
			},
			want: encoder.Vector{30: 3.0 / 15, 32: 4.0 / 15, 35: 5.0 / 15, 48: 1, 49: 3.0 / 15, 51: 1, 54: 1},
		},
		{
			name: "Black terminal victory",
			position: game.Position{
				Ruleset:  game.Ruleset,
				Checkers: [2][24]int{{18: 1, 19: 2, 20: 3, 21: 4, 23: 5}, {}},
				BorneOff: [2]int{0, 15},
				Turn:     game.White, Starter: game.Black, FirstDone: [2]bool{true, true},
			},
			want: encoder.Vector{18: 1.0 / 15, 19: 2.0 / 15, 20: 3.0 / 15, 21: 4.0 / 15, 23: 5.0 / 15, 49: 1, 50: 1, 55: 1},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := test.position
			got, err := encoder.Encode(test.position)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("want %v, got %v", test.want, got)
			}
			if len(got) != 56 {
				t.Fatalf("want 56 values, got %d", len(got))
			}
			for index, value := range got {
				if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
					t.Fatalf("input %d is not finite and normalized: %v", index, value)
				}
			}
			if test.position != before {
				t.Fatal("encoding mutated the input")
			}
			data, err := json.Marshal(test.position)
			if err != nil {
				t.Fatal(err)
			}
			var restored game.Position
			if err := json.Unmarshal(data, &restored); err != nil {
				t.Fatal(err)
			}
			for range 3 {
				encoded, err := encoder.Encode(restored)
				if err != nil || encoded != got {
					t.Fatalf("equivalent public position encoded differently: %v, %v", encoded, err)
				}
			}
		})
	}
}

func TestEncodeCompletedOpeningTurns(t *testing.T) {
	for _, test := range []struct {
		player game.Player
		turn   game.Turn
		want   encoder.Vector
	}{
		{game.White, game.Turn{Steps: []game.Step{{From: 0, To: 1, Die: 1}, {From: 1, To: 3, Die: 2}}}, encoder.Vector{0: 14.0 / 15, 3: 1.0 / 15, 36: 1, 51: 1, 53: 1, 54: 1}},
		{game.Black, game.Turn{Steps: []game.Step{{From: 12, To: 13, Die: 1}, {From: 13, To: 15, Die: 2}}}, encoder.Vector{0: 1, 36: 14.0 / 15, 39: 1.0 / 15, 50: 1, 52: 1, 55: 1}},
	} {
		next, err := game.ApplyTurn(game.Initial(test.player), game.Dice{1, 2}, test.turn)
		if err != nil {
			t.Fatal(err)
		}
		got, err := encoder.Encode(next)
		if err != nil || got != test.want {
			t.Fatalf("completed opening by player %d: want %v, got %v, %v", test.player, test.want, got, err)
		}
	}
}

func TestTurnStarterAndOpeningFlagsAreIndependentInputs(t *testing.T) {
	p := game.Initial(game.White)
	p.FirstDone = [2]bool{true, true}
	before, err := encoder.Encode(p)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name   string
		change func(*game.Position)
		want   encoder.Vector
	}{
		{"turn", func(p *game.Position) { p.Turn = game.Black }, encoder.Vector{0: 1, 36: 1, 51: 1, 54: 1}},
		{"starter", func(p *game.Position) { p.Starter = game.Black }, encoder.Vector{0: 1, 36: 1, 50: 1, 55: 1}},
		{"White first turn pending", func(p *game.Position) { p.FirstDone[game.White] = false }, encoder.Vector{0: 1, 36: 1, 50: 1, 52: 1, 54: 1}},
		{"Black first turn pending", func(p *game.Position) { p.FirstDone[game.Black] = false }, encoder.Vector{0: 1, 36: 1, 50: 1, 53: 1, 54: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			q := p
			test.change(&q)
			got, err := encoder.Encode(q)
			if err != nil || got != test.want || got == before {
				t.Fatalf("want %v, got %v, %v", test.want, got, err)
			}
		})
	}
}

func TestRejectInvalidPosition(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*game.Position)
	}{
		{"ruleset", func(p *game.Position) { p.Ruleset = "unsupported" }},
		{"negative turn", func(p *game.Position) { p.Turn = -1 }},
		{"turn", func(p *game.Position) { p.Turn = 2 }},
		{"starter", func(p *game.Position) { p.Starter = 2 }},
		{"negative count", func(p *game.Position) { p.Checkers[game.White][0] = -1 }},
		{"missing checker", func(p *game.Position) { p.Checkers[game.White][0] = 14 }},
		{"borne-off count", func(p *game.Position) { p.BorneOff[game.Black] = 16 }},
		{"overlap", func(p *game.Position) { p.Checkers[game.Black][12] = 14; p.Checkers[game.Black][0] = 1 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := game.Initial(game.White)
			test.change(&p)
			before := p
			got, err := encoder.Encode(p)
			if err == nil || got != (encoder.Vector{}) || p != before {
				t.Fatalf("invalid position must fail without partial output or mutation: %v, %v", got, err)
			}
		})
	}
}
