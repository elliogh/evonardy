package research

import (
	"evonardy/internal/game"
	"evonardy/internal/replay"
	"evonardy/internal/training"
	"reflect"
	"testing"
)

func fixture(n int, win func(int, int) bool) []training.Score {
	out := []training.Score{}
	for i := 0; i < n; i++ {
		for side := 0; side < 2; side++ {
			s := game.Player(side)
			winner := s.Other()
			if win(i, side) {
				winner = s
			}
			out = append(out, training.Score{Index: len(out), Seed: uint64(i), Side: s, OpponentID: "incumbent", Status: replay.Completed, Outcome: &game.Outcome{Winner: winner, Points: 1}, Turns: 20})
		}
	}
	return out
}
func TestPairsStayTogether(t *testing.T) {
	scores := fixture(40, func(i, s int) bool { return s == 0 })
	e, err := Analyze([][]training.Score{scores}, DefaultBootstrap())
	if err != nil {
		t.Fatal(err)
	}
	if e.WinRate != .5 || e.WinInterval != (Interval{.5, .5}) || e.Games != 80 {
		t.Fatalf("correlated sides were separated: %+v", e)
	}
	p := DefaultConfirmation()
	p.Pairs = 40
	p.MinimumPairs = 30
	v, err := Confirm(scores, p)
	if err != nil || v.Status != "candidate" {
		t.Fatalf("degenerate confirmation %+v %v", v, err)
	}
}
func TestSeedVariabilityAndPairedDifferences(t *testing.T) {
	lose := fixture(10, func(i, s int) bool { return false })
	win := fixture(10, func(i, s int) bool { return true })
	runs := [][]training.Score{lose, win}
	e, err := Analyze(runs, DefaultBootstrap())
	if err != nil {
		t.Fatal(err)
	}
	if e.WinInterval.Low != 0 || e.WinInterval.High != 1 {
		t.Fatalf("lost between-seed variation %+v", e)
	}
	same, err := Compare(runs, runs, DefaultBootstrap())
	if err != nil || same.WinInterval != (Interval{0, 0}) {
		t.Fatalf("unpaired comparison %+v %v", same, err)
	}
	again, err := Analyze(runs, DefaultBootstrap())
	if err != nil || !reflect.DeepEqual(e, again) {
		t.Fatal("bootstrap not deterministic")
	}
	win = fixture(10, func(i, s int) bool { return true })
	win[0].Seed = 999
	if _, err := Compare(runs, [][]training.Score{lose, win}, DefaultBootstrap()); err == nil {
		t.Fatal("accepted mismatched schedule")
	}
}
func TestInvalidPairsAndConfirmation(t *testing.T) {
	for _, kind := range []string{"missing", "seed", "side", "truncated", "duplicate", "outcome"} {
		t.Run(kind, func(t *testing.T) {
			s := fixture(3, func(i, j int) bool { return true })
			switch kind {
			case "missing":
				s = s[:5]
			case "seed":
				s[1].Seed++
			case "side":
				s[1].Side = game.White
			case "truncated":
				s[1].Status = replay.Truncated
				s[1].Outcome = nil
				s[1].Turns = replay.MaxEvents
			case "duplicate":
				s[2].Seed = 0
				s[3].Seed = 0
			case "outcome":
				s[1].Outcome.Points = 9
			}
			if _, err := Analyze([][]training.Score{s}, DefaultBootstrap()); err == nil {
				t.Fatal("accepted malformed independent pairs")
			}
		})
	}
	p := DefaultConfirmation()
	p.Pairs = 100
	s := fixture(100, func(i, j int) bool { return i%10 != 0 })
	v, err := Confirm(s, p)
	if err != nil || v.Status != "confirmed" {
		t.Fatalf("expected confirmed %+v %v", v, err)
	}
	p.Pairs = 2
	v, err = Confirm(s[:4], p)
	if err != nil || v.Status != "candidate" {
		t.Fatalf("smoke cannot confirm %+v %v", v, err)
	}
	if _, err := Confirm(s, p); err == nil {
		t.Fatal("accepted changed final budget")
	}
}
