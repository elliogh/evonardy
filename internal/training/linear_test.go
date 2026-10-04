package training_test

import (
	"context"
	"encoding/json"
	"errors"
	"evonardy/internal/training"
	"reflect"
	"testing"
)

func small() training.Config {
	c := training.DefaultConfig()
	c.Population = 4
	c.Generations = 2
	c.PairsPerOpponent = 1
	c.Workers = 2
	return c
}
func complete(t *testing.T, s training.State) training.State {
	t.Helper()
	for s.Generation < s.Config.Generations {
		tasks := training.NextTasks(s)
		scores, err := training.Play(context.Background(), s, tasks)
		if err != nil {
			t.Fatal(err)
		}
		s, err = training.Add(s, scores)
		if err != nil {
			t.Fatal(err)
		}
		if training.Ready(s) {
			s, _, err = training.Advance(s)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	return s
}
func TestGenerationUsesWholeGamesAndResumeMatchesUninterrupted(t *testing.T) {
	c := small()
	initial, err := training.New(c)
	if err != nil {
		t.Fatal(err)
	}
	full := complete(t, initial)
	tasks := training.NextTasks(initial)
	scores, err := training.Play(context.Background(), initial, tasks)
	if err != nil {
		t.Fatal(err)
	}
	partial, err := training.Add(initial, scores)
	if err != nil {
		t.Fatal(err)
	}
	if partial.Generation != 0 || len(partial.History) != 0 {
		t.Fatal("selected an incomplete generation")
	}
	data, err := json.Marshal(partial)
	if err != nil {
		t.Fatal(err)
	}
	var loaded training.State
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if err := training.Validate(loaded); err != nil {
		t.Fatal(err)
	}
	resumed := complete(t, loaded)
	if !reflect.DeepEqual(full, resumed) {
		t.Fatal("checkpoint resume changed selection, genomes, or counters")
	}
	if full.Counters.Games != 32 || full.Counters.Mutations == 0 || full.Counters.ForwardEvaluations == 0 {
		t.Fatalf("missing actual work: %+v", full.Counters)
	}
	changed := false
	for i := range initial.Population {
		if initial.Population[i].Weights != full.Population[i].Weights {
			changed = true
		}
	}
	if !changed {
		t.Fatal("GA never updated coefficients")
	}
	c.Workers = 1
	serial, err := training.New(c)
	if err != nil {
		t.Fatal(err)
	}
	serial = complete(t, serial)
	if !reflect.DeepEqual(serial.Population, full.Population) || !reflect.DeepEqual(serial.History, full.History) || serial.Counters != full.Counters {
		t.Fatal("worker scheduling changed selection")
	}
}
func TestTruncationCannotProduceFitnessOrSelection(t *testing.T) {
	c := small()
	c.MaxTurns = 1
	s, err := training.New(c)
	if err != nil {
		t.Fatal(err)
	}
	scores, err := training.Play(context.Background(), s, training.NextTasks(s))
	if err != nil {
		t.Fatal(err)
	}
	after, err := training.Add(s, scores)
	if !errors.Is(err, training.ErrTruncated) {
		t.Fatal("truncation was assigned a reward")
	}
	if after.Generation != 0 || len(after.History) != 0 || training.Ready(after) {
		t.Fatal("incomplete evaluation selected parents")
	}
	if _, _, err := training.Advance(after); err == nil {
		t.Fatal("advanced after a truncated game")
	}
	for _, candidate := range after.Population {
		if candidate.Stats != nil {
			t.Fatal("invented fitness for an incomplete generation")
		}
	}
}
