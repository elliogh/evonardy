package game_test

import (
	"encoding/json"
	"slices"
	"testing"

	"evonardy/internal/game"
)

// Independent reference solver: progress coordinates, explicit six-point windows,
// raw dice permutations, no engine legality/application calls or search memoization.
// It is intentionally slow and used only for bounded correctness fixtures.
func referencePaths(p game.Position, dice game.Dice) []game.Turn {
	var own, enemy [24]int
	for i := 0; i < 24; i++ {
		physical := (int(p.Turn)*12 + i) % 24
		own[i] = p.Checkers[p.Turn][physical]
		enemy[i] = p.Checkers[p.Turn.Other()][physical]
	}
	values := []int{dice[0], dice[1]}
	if dice[0] == dice[1] {
		values = append(values, dice[0], dice[0])
	}
	headLimit := 1
	if !p.FirstDone[p.Turn] && p.Turn != p.Starter && dice[0] == dice[1] && (dice[0] == 3 || dice[0] == 4 || dice[0] == 6) {
		headLimit = 2
	}
	type leaf struct {
		steps    []game.Step
		terminal bool
	}
	var leaves []leaf
	var enumerate func([24]int, []int, int, []game.Step)
	enumerate = func(board [24]int, remaining []int, taken int, steps []game.Step) {
		total := 0
		for _, count := range board {
			total += count
		}
		if total == 0 {
			leaves = append(leaves, leaf{append([]game.Step{}, steps...), true})
			return
		}
		branches := 0
		for k, die := range remaining {
			if k > 0 && remaining[k-1] == die {
				continue
			}
			for from, count := range board {
				if count == 0 || (from == 0 && taken == headLimit) {
					continue
				}
				to := from + die
				if to < 24 {
					if enemy[to] != 0 {
						continue
					}
				} else {
					canOff := true
					for i, n := range board {
						if n > 0 && (i < 18 || (to > 24 && i < from)) {
							canOff = false
						}
					}
					if !canOff {
						continue
					}
				}
				next := board
				next[from]--
				if to < 24 {
					next[to]++
				}
				blocked := false
				if p.BorneOff[p.Turn.Other()] == 0 {
					for start := 0; start <= 18; start++ {
						window, behind := true, true
						for i := start; i < start+6; i++ {
							if next[(i+12)%24] == 0 {
								window = false
							}
						}
						for i := start; i < 24; i++ {
							if enemy[(i+12)%24] > 0 {
								behind = false
							}
						}
						if window && behind {
							blocked = true
						}
					}
				}
				if blocked {
					continue
				}
				branches++
				physicalFrom := (int(p.Turn)*12 + from) % 24
				physicalTo := game.Off
				if to < 24 {
					physicalTo = (int(p.Turn)*12 + to) % 24
				}
				left := append([]int{}, remaining[:k]...)
				left = append(left, remaining[k+1:]...)
				head := taken
				if from == 0 {
					head++
				}
				path := append(append([]game.Step{}, steps...), game.Step{From: physicalFrom, To: physicalTo, Die: die})
				enumerate(next, left, head, path)
			}
		}
		if branches == 0 {
			leaves = append(leaves, leaf{append([]game.Step{}, steps...), false})
		}
	}
	slices.Sort(values)
	enumerate(own, values, 0, nil)
	maximum, larger := 0, 0
	for _, l := range leaves {
		maximum = max(maximum, len(l.steps))
	}
	if maximum == 1 && dice[0] != dice[1] {
		for _, l := range leaves {
			if len(l.steps) == 1 {
				larger = max(larger, l.steps[0].Die)
			}
		}
	}
	result := []game.Turn{}
	for _, l := range leaves {
		if !l.terminal && len(l.steps) < maximum {
			continue
		}
		if larger > 0 && len(l.steps) == 1 && l.steps[0].Die < larger {
			continue
		}
		result = append(result, game.Turn{Steps: l.steps})
	}
	return result
}

func pathKeys(paths []game.Turn) []string {
	keys := make([]string, 0, len(paths))
	for _, path := range paths {
		data, _ := json.Marshal(path)
		keys = append(keys, string(data))
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

func TestGeneratorCompletenessAgainstIndependentOracle(t *testing.T) {
	cases := fixtures(t)
	cases = append(cases, fixture{Name: "mixed_home", Own: map[string]int{"18": 1, "20": 1, "23": 1}, Opponent: map[string]int{"0": 15}, Dice: game.Dice{2, 4}},
		fixture{Name: "split_midgame", Own: map[string]int{"2": 2, "8": 1, "17": 1}, Opponent: map[string]int{"0": 12, "21": 1, "22": 1, "23": 1}, Dice: game.Dice{1, 1}},
		fixture{Name: "intermediate_block", Own: map[string]int{"14": 10, "15": 1, "16": 1, "17": 1, "19": 1, "20": 1}, Opponent: map[string]int{"0": 15}, Dice: game.Dice{4, 1}},
		fixture{Name: "crossing_zero_block", Own: map[string]int{"0": 11, "1": 1, "3": 1, "22": 1, "23": 1}, Opponent: map[string]int{"8": 15}, Dice: game.Dice{2, 1}})
	for _, f := range cases {
		for _, side := range []game.Player{game.White, game.Black} {
			p := fixturePosition(t, f, side)
			actual, err := game.LegalPaths(p, f.Dice)
			if err != nil {
				t.Fatal(err)
			}
			expected := referencePaths(p, f.Dice)
			if !slices.Equal(pathKeys(actual), pathKeys(expected)) {
				t.Fatalf("%s side %d\nactual: %v\nreference: %v", f.Name, side, pathKeys(actual), pathKeys(expected))
			}
			unique, err := game.LegalTurns(p, f.Dice)
			if err != nil {
				t.Fatal(err)
			}
			allStates, uniqueStates := map[game.Position]bool{}, map[game.Position]bool{}
			for _, path := range actual {
				q, err := game.ApplyTurn(p, f.Dice, path)
				if err != nil {
					t.Fatal(err)
				}
				allStates[q] = true
			}
			for _, path := range unique {
				q, err := game.ApplyTurn(p, f.Dice, path)
				if err != nil {
					t.Fatal(err)
				}
				uniqueStates[q] = true
			}
			if len(unique) != len(allStates) || len(uniqueStates) != len(allStates) {
				t.Fatalf("%s: final-state dedup lost states", f.Name)
			}
			for state := range allStates {
				if !uniqueStates[state] {
					t.Fatalf("%s: missing state", f.Name)
				}
			}
		}
	}
}

func FuzzLegalTurnInvariants(f *testing.F) {
	f.Add([]byte{1, 2, 3, 4, 5, 6, 2, 2})
	f.Add([]byte{6, 6, 4, 4, 3, 3, 1, 1})
	f.Fuzz(func(t *testing.T, data []byte) {
		p := game.Initial(game.White)
		if len(data) > 48 {
			data = data[:48]
		}
		for i := 0; i+1 < len(data); i += 2 {
			dice := game.Dice{int(data[i]%6) + 1, int(data[i+1]%6) + 1}
			before := p
			turns, err := game.LegalTurns(p, dice)
			if err != nil {
				t.Fatal(err)
			}
			for _, turn := range turns {
				q, err := game.ApplyTurn(p, dice, turn)
				if err != nil {
					t.Fatal(err)
				}
				if err := game.ValidatePosition(q); err != nil {
					t.Fatal(err)
				}
				for _, step := range turn.Steps {
					if step.To != game.Off && game.Progress(p.Turn, step.To) <= game.Progress(p.Turn, step.From) {
						t.Fatal("non-increasing progress")
					}
				}
			}
			if p != before {
				t.Fatal("input mutated")
			}
			if len(turns) == 0 {
				break
			}
			p, err = game.ApplyTurn(p, dice, turns[int(data[i])%len(turns)])
			if err != nil {
				t.Fatal(err)
			}
		}
	})
}

func BenchmarkLegalTurns(b *testing.B) {
	p := fixturePosition(b, fixture{Name: "bench", Own: map[string]int{"0": 8, "2": 2, "5": 2, "8": 1, "14": 1, "19": 1}, Opponent: map[string]int{"0": 10, "3": 2, "9": 2, "18": 1}}, game.White)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := game.LegalTurns(p, game.Dice{3, 3}); err != nil {
			b.Fatal(err)
		}
	}
}
