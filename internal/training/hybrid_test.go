package training

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func hybridFixture(t *testing.T) NeuroState {
	t.Helper()
	c := DefaultHybridConfig()
	c.Rounds = 2
	c.GamesPerRound = 1
	c.MaxTurns = 2
	s, err := NewHybrid(c)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func trainHybridRound(t *testing.T, s NeuroState) NeuroState {
	t.Helper()
	for !HybridTrainingReady(s) {
		next, _, err := TrainHybridGame(context.Background(), s)
		if err != nil {
			t.Fatal(err)
		}
		s = next
	}
	return s
}
func TestHybridCopiesTrainedParentsAndAccountsDiscardedWork(t *testing.T) {
	initial := hybridFixture(t)
	trained := trainHybridRound(t, initial)
	if trained.Counters.Games != 8 || trained.Counters.Updates != 16 || trained.Counters.TruncatedGames != 8 {
		t.Fatal(trained.Counters)
	}
	if trained.Population[0].Parameters == initial.Population[0].Parameters {
		t.Fatal("participant did not learn")
	}
	scored := fillNeuro(t, trained)
	before, _ := json.Marshal(scored)
	next, gen, err := AdvanceHybrid(scored)
	if err != nil {
		t.Fatal(err)
	}
	if len(gen.Replacements) != 2 || gen.TrainingCounters.Updates != 16 || next.Counters.Games != 40 || next.Counters.Updates != 16 {
		t.Fatal("discarded work omitted", next.Counters)
	}
	for i, r := range gen.Replacements {
		child := next.Population[6+i]
		parent := gen.Ranked[i]
		if child.Parameters != parent.Parameters || r.ParentID != parent.ID || r.ReplacedID != gen.Ranked[6+i].ID || r.ChildID != child.ID || r.After != child.Hyperparameters || r.Before != parent.Hyperparameters {
			t.Fatal("replacement did not copy trained parent", r)
		}
		if child.Hyperparameters == parent.Hyperparameters {
			t.Fatal("no adaptation")
		}
	}
	if err := ValidateNeuro(next); err != nil {
		t.Fatal(err)
	}
	resumed := trainHybridRound(t, next)
	if resumed.Counters.Updates != 32 || resumed.Counters.Games != 48 {
		t.Fatal(resumed.Counters)
	}
	after, _ := json.Marshal(scored)
	if string(before) != string(after) {
		t.Fatal("selection or replacement mutated parents")
	}
	bad := next
	bad.Counters.Updates--
	if ValidateNeuro(bad) == nil {
		t.Fatal("accepted omitted updates")
	}
}
func TestHybridSerializedResumeAndTruncatedSelection(t *testing.T) {
	s := hybridFixture(t)
	partial, played, err := TrainHybridGame(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if played.Replay.Bots[0] != s.Population[0].ID {
		t.Fatal("wrong learner identity")
	}
	data, _ := json.Marshal(partial)
	var restored NeuroState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	a := trainHybridRound(t, partial)
	b := trainHybridRound(t, restored)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("resumed learner weights or counters differ")
	}
	failed, gen, _, err := StepNeuro(context.Background(), a)
	if !errors.Is(err, ErrTruncated) || gen != nil || failed.Counters.Games != 10 || failed.Counters.Updates != 16 || failed.Generation != 0 {
		t.Fatal("partial selection replaced population", err)
	}
	if err := ValidateNeuro(failed); err != nil {
		t.Fatal(err)
	}
	if failed.Population[0].Parameters != a.Population[0].Parameters {
		t.Fatal("evaluation trained learner")
	}
}
func TestHybridBoundsAndPhase(t *testing.T) {
	c := DefaultHybridConfig()
	c.AlphaBounds.Min = 0
	if _, err := NewHybrid(c); err == nil {
		t.Fatal("zero alpha bound accepted")
	}
	s := hybridFixture(t)
	if _, _, err := AdvanceHybrid(s); err == nil {
		t.Fatal("selected before learners finished")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	next, _, err := TrainHybridGame(ctx, s)
	if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(next, s) {
		t.Fatal("partial game committed", err)
	}
}
