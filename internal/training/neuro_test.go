package training

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"evonardy/internal/game"
	"evonardy/internal/neural"
	"evonardy/internal/replay"
)

func neuroFixture(t *testing.T) NeuroState {
	t.Helper()
	c := DefaultGAMLPConfig()
	c.Population = 4
	c.Generations = 2
	c.PairsPerOpponent = 1
	c.Workers = 2
	c.MaxTurns = 2
	s, err := NewGAMLP(c)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func fillNeuro(t *testing.T, s NeuroState) NeuroState {
	t.Helper()
	scores := []Score{}
	for i := 0; i < NeuroSlots(s); i++ {
		x := task(s.schedule(), i)
		winner := x.Side
		if x.Candidate > 0 {
			winner = winner.Other()
		}
		scores = append(scores, Score{Index: i, Seed: x.Seed, Side: x.Side, OpponentID: s.Development[x.Opponent].ID, Status: replay.Completed, Outcome: &game.Outcome{Winner: winner, Points: 1}, Turns: 1, ForwardEvaluations: 3})
	}
	next, err := AddNeuro(s, scores)
	if err != nil {
		t.Fatal(err)
	}
	return next
}
func TestNeuroEvolutionAndOwnedSnapshots(t *testing.T) {
	s := fillNeuro(t, neuroFixture(t))
	before, _ := json.Marshal(s)
	next, gen, err := AdvanceNeuro(s)
	if err != nil {
		t.Fatal(err)
	}
	if gen.Ranked[0].ID != s.Population[0].ID || gen.Metric.BestFitness != 1 || next.Population[0].Parameters != s.Population[0].Parameters {
		t.Fatal("wrong ranking or elite")
	}
	if next.Counters.Updates != 0 || next.Counters.Crossovers != 0 || next.Counters.Mutations != 3*neural.ParameterCount {
		t.Fatal(next.Counters)
	}
	for _, p := range next.Population {
		if len(p.Parents) != 1 {
			t.Fatal("mutation-only ancestry")
		}
	}
	if err := ValidateNeuro(next); err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(next)
	var restored NeuroState
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatal(err)
	}
	a, ga, err := AdvanceNeuro(fillNeuro(t, next))
	if err != nil {
		t.Fatal(err)
	}
	b, gb, err := AdvanceNeuro(fillNeuro(t, restored))
	if err != nil || !reflect.DeepEqual(a, b) || !reflect.DeepEqual(ga, gb) {
		t.Fatal("serialized continuation diverged", err)
	}
	next.Population[0].Parameters[0] = 900
	gen.Ranked[0].Parents = append(gen.Ranked[0].Parents, "changed")
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("parent mutated")
	}
	bad := a
	bad.Counters.Updates++
	if ValidateNeuro(bad) == nil {
		t.Fatal("accepted gradient update in GA")
	}
}
func TestNeuroWorkersAndTruncatedSelection(t *testing.T) {
	s := neuroFixture(t)
	wave, err := PlayNeuroWave(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	serial := s
	serial.Config.Workers = 1
	first, err := PlayNeuroWave(context.Background(), serial)
	if err != nil {
		t.Fatal(err)
	}
	serial, err = AddNeuro(serial, first.Scores)
	if !errors.Is(err, ErrTruncated) {
		t.Fatal(err)
	}
	second, err := PlayNeuroWave(context.Background(), serial)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wave.Scores, append(first.Scores, second.Scores...)) {
		t.Fatal("worker-dependent dice or scores")
	}
	if _, err := replay.Play(wave.Sample.Replay); err != nil {
		t.Fatal(err)
	}
	failed, err := AddNeuro(s, wave.Scores)
	if !errors.Is(err, ErrTruncated) || failed.Counters.Games != 2 || failed.Counters.TruncatedGames != 2 {
		t.Fatal(err, failed.Counters)
	}
	if _, _, err := AdvanceNeuro(failed); err == nil {
		t.Fatal("selected on partial fitness")
	}
	if err := ValidateNeuro(failed); err != nil {
		t.Fatal(err)
	}
	if s.Counters.Games != 0 || len(s.Results) != 0 {
		t.Fatal("input modified")
	}
}
