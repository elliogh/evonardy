package game_test

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"evonardy/internal/game"
)

type fixture struct {
	Name            string         `json:"name"`
	Own             map[string]int `json:"own"`
	Opponent        map[string]int `json:"opponent"`
	Dice            game.Dice      `json:"dice"`
	Moves           int            `json:"moves"`
	SecondFirst     bool           `json:"second_first"`
	MaxHeads        int            `json:"max_heads"`
	OnlyDie         int            `json:"only_die"`
	Terminal        bool           `json:"terminal"`
	TerminalShorter bool           `json:"terminal_shorter"`
}

func fixtures(t testing.TB) []fixture {
	t.Helper()
	data, err := os.ReadFile("../../testdata/positions/rules.json")
	if err != nil {
		t.Fatal(err)
	}
	var result []fixture
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func fixturePosition(t testing.TB, f fixture, player game.Player) game.Position {
	t.Helper()
	p := game.Initial(player)
	p.Checkers = [2][24]int{}
	p.BorneOff = [2]int{15, 15}
	p.FirstDone = [2]bool{true, true}
	if f.SecondFirst {
		p.Starter = player.Other()
		p.FirstDone[player] = false
	}
	for side, pieces := range []map[string]int{f.Own, f.Opponent} {
		owner := player
		if side == 1 {
			owner = player.Other()
		}
		for raw, count := range pieces {
			progress, err := strconv.Atoi(raw)
			if err != nil {
				t.Fatal(err)
			}
			p.Checkers[owner][game.PhysicalPoint(owner, progress)] = count
			p.BorneOff[owner] -= count
		}
	}
	if err := game.ValidatePosition(p); err != nil {
		t.Fatalf("fixture %s: %v", f.Name, err)
	}
	return p
}

func TestRulesFixturesBothSides(t *testing.T) {
	for _, f := range fixtures(t) {
		for _, player := range []game.Player{game.White, game.Black} {
			t.Run(f.Name+"/"+strconv.Itoa(int(player)), func(t *testing.T) {
				p := fixturePosition(t, f, player)
				before := p
				paths, err := game.LegalPaths(p, f.Dice)
				if err != nil {
					t.Fatal(err)
				}
				most, heads, short := 0, 0, false
				for _, path := range paths {
					most = max(most, len(path.Steps))
					count := 0
					for _, step := range path.Steps {
						if step.From == game.Head(player) {
							count++
						}
						if f.OnlyDie != 0 && step.Die != f.OnlyDie {
							t.Fatalf("wrong die: %+v", path)
						}
					}
					heads = max(heads, count)
					next, err := game.ApplyTurn(p, f.Dice, path)
					if err != nil {
						t.Fatalf("generated turn rejected: %+v: %v", path, err)
					}
					if err := game.ValidatePosition(next); err != nil {
						t.Fatal(err)
					}
					_, terminal := game.Result(next)
					if f.Terminal && !terminal {
						t.Fatalf("expected victory: %+v", path)
					}
					if terminal && len(path.Steps) < f.Moves {
						short = true
					}
				}
				if most != f.Moves {
					t.Fatalf("want %d moves, got %d: %+v", f.Moves, most, paths)
				}
				if f.MaxHeads != 0 && heads != f.MaxHeads {
					t.Fatalf("want head maximum %d, got %d", f.MaxHeads, heads)
				}
				if f.TerminalShorter && !short {
					t.Fatal("immediate terminal path lost")
				}
				if p != before {
					t.Fatal("input mutated")
				}
			})
		}
	}
}
