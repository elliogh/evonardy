package training_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"evonardy/internal/agent"
	"evonardy/internal/features"
	"evonardy/internal/training"
)

func linearSource() training.LinearSource {
	w := features.DefaultWeights
	w[3], w[7] = 1.6, .95
	return training.LinearSource{Policy: agent.Policy{ID: strings.Repeat("a", 64), Kind: "linear", Weights: w}, ModelSHA256: strings.Repeat("b", 64)}
}

func TestSavedSourceInitializationPairingAndResume(t *testing.T) {
	c := small()
	source := linearSource()
	initial, err := training.NewFromModel(c, source)
	if err != nil {
		t.Fatal(err)
	}
	if initial.Population[0].Weights != source.Policy.Weights || initial.Population[1].Weights == source.Policy.Weights || training.Slots(initial) != 24 || initial.Algorithm != training.FromModelAlgorithm {
		t.Fatal("invalid source population or budget")
	}
	source.Policy.Weights[0] = 9
	if initial.Source.Policy.Weights == source.Policy.Weights {
		t.Fatal("source aliases caller data")
	}
	firstSeed := training.NextTasks(initial)[0].Seed
	if firstSeed == training.PairSeed(c.Seed, "ga/development", 0, 0) {
		t.Fatal("reused legacy schedule")
	}
	full := complete(t, initial)
	if full.Counters.Games != 48 || full.Counters.TruncatedGames != 0 {
		t.Fatal("wrong three-opponent work")
	}
	partial := initial
	for partial.Generation == 0 {
		tasks := training.NextTasks(partial)
		for _, task := range tasks {
			if task.Seed != training.SelectionPairSeed(initial, 0, task.Opponent, task.Pair) {
				t.Fatal("candidate-dependent or side-dependent dice")
			}
		}
		scores, err := training.Play(context.Background(), partial, tasks)
		if err != nil {
			t.Fatal(err)
		}
		partial, err = training.Add(partial, scores)
		if err != nil {
			t.Fatal(err)
		}
		if training.Ready(partial) {
			var gen training.Generation
			partial, gen, err = training.Advance(partial)
			if err != nil || gen.SourceBotID != initial.Source.Policy.ID || gen.Algorithm != training.FromModelAlgorithm {
				t.Fatal("missing generation provenance", err)
			}
		}
	}
	if training.NextTasks(partial)[0].Seed == firstSeed {
		t.Fatal("generation reused dice")
	}
	data, _ := json.Marshal(partial)
	var loaded training.State
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	if err := training.Validate(loaded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(full, complete(t, loaded)) {
		t.Fatal("saved-source resume changed evolution")
	}
	c.Workers = 1
	one, err := training.NewFromModel(c, linearSource())
	if err != nil {
		t.Fatal(err)
	}
	one = complete(t, one)
	one.Config.Workers = full.Config.Workers
	if !reflect.DeepEqual(one, full) {
		t.Fatal("worker count changed saved-source evolution")
	}
}

func TestSourceContractsAndThreeOpponentCapacity(t *testing.T) {
	c := training.DefaultConfig()
	c.Generations, c.PairsPerOpponent = 150, 4
	if _, err := training.New(c); err != nil {
		t.Fatal("legacy budget should fit", err)
	}
	if _, err := training.NewFromModel(c, linearSource()); err == nil {
		t.Fatal("accepted oversized three-opponent budget")
	}
	s, _ := training.NewFromModel(small(), linearSource())
	s.RandomContract = training.RandomContract
	if training.Validate(s) == nil {
		t.Fatal("accepted changed source contract")
	}
	source := linearSource()
	source.Policy.Weights[0] = 11
	if _, err := training.NewFromModel(small(), source); err == nil {
		t.Fatal("clipped source instead of preserving it")
	}
}
