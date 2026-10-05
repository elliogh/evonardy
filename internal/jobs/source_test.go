package jobs_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"evonardy/internal/agent"
	"evonardy/internal/features"
	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/training"
)

func TestSavedSourceRetryStopReopenAndDescendant(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, bots, m := open(t, dir)
	weights := features.DefaultWeights
	weights[7] = .95
	card, err := bots.SaveLinear("Source", weights[:], "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join("bots", card.ID, "model.json")
	before, err := store.Read(modelPath, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	req := jobs.StartRequest{Command: library.Command{CommandID: "from-source"}, Name: "Refinement", Config: config(), SourceBotID: card.ID}
	x, err := m.Start(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if x.SourceBotID != card.ID || x.SourceModelSHA256 == "" || x.Algorithm != training.FromModelAlgorithm || x.GenerationBudget != 24 {
		t.Fatal("missing source or budget", x)
	}
	if _, err = m.Start(ctx, req); err != nil {
		t.Fatal("retry failed", err)
	}
	changed := req
	changed.SourceBotID = "another-source"
	if _, err = m.Start(ctx, changed); err != jobs.ErrConflict {
		t.Fatal("changed-source retry did not conflict", err)
	}
	if _, err = m.Stop(ctx, x.ID, library.Command{CommandID: "stop-source", ExpectedVersion: x.Version}); err != nil {
		t.Fatal(err)
	}
	stopped := wait(t, m, x.ID)
	if stopped.State != jobs.Stopped {
		t.Fatal(stopped.State)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	after, _ := store.Read(modelPath, 1<<20)
	if !bytes.Equal(before, after) {
		t.Fatal("training changed source")
	}
	store.Close()
	// The frozen source must suffice even if its library package is unavailable.
	backup := filepath.Join(dir, "preserved-source")
	if err = os.Rename(filepath.Join(dir, "bots", card.ID), backup); err != nil {
		t.Fatal(err)
	}
	store, bots, m = open(t, dir)
	defer store.Close()
	defer m.Close()
	if _, err = m.Start(ctx, req); err != nil {
		t.Fatal("retry reloaded missing source", err)
	}
	x, err = m.Resume(ctx, x.ID, library.Command{CommandID: "resume-source", ExpectedVersion: stopped.Version})
	if err != nil {
		t.Fatal(err)
	}
	done := wait(t, m, x.ID)
	if done.State != jobs.Completed || done.Counters.Games != 72 || done.SourceBotID != card.ID {
		t.Fatal("lost source or work", done)
	}
	initial, err := training.NewFromModel(config(), training.LinearSource{Policy: agent.Policy{ID: card.ID, Kind: "linear", Weights: weights}, ModelSHA256: done.SourceModelSHA256})
	if err != nil {
		t.Fatal(err)
	}
	for initial.Generation < initial.Config.Generations {
		scores, err := training.Play(ctx, initial, training.NextTasks(initial))
		if err != nil {
			t.Fatal(err)
		}
		initial, err = training.Add(initial, scores)
		if err != nil {
			t.Fatal(err)
		}
		if training.Ready(initial) {
			initial, _, err = training.Advance(initial)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if !reflect.DeepEqual(done.Candidates, initial.Population) || !reflect.DeepEqual(done.History, initial.History) || done.Counters != initial.Counters {
		t.Fatal("durable resume changed source evolution")
	}
	gen, err := m.Generation(done.ID, 1)
	if err != nil || gen.SourceBotID != card.ID {
		t.Fatal("missing generation provenance", err)
	}
	watched, err := m.Watch(done.ID)
	if err != nil || watched.OpponentID != card.ID {
		t.Fatal("source opponent replay invalid", err)
	}
	var candidate training.Candidate
	for _, c := range done.Candidates {
		if c.Weights != weights {
			candidate = c
			break
		}
	}
	if candidate.ID == "" {
		t.Fatal("no changed descendant")
	}
	descendant, err := m.Save(ctx, done.ID, jobs.SaveRequest{Command: library.Command{CommandID: "save-source", ExpectedVersion: done.Version}, Generation: done.Generation, CandidateID: candidate.ID, Name: "Descendant"})
	if err != nil {
		t.Fatal(err)
	}
	detail, err := bots.Get(descendant.ID)
	if err != nil || !strings.Contains(detail.Source, "trained from "+card.ID) || !strings.Contains(detail.Source, done.SourceModelSHA256) {
		t.Fatal("missing descendant lineage", err)
	}
	frozen, err := bots.Freeze(descendant.ID)
	if err != nil || frozen.Weights != candidate.Weights {
		t.Fatal("saved wrong weights", err)
	}
	original, err := os.ReadFile(filepath.Join(backup, "model.json"))
	if err != nil || !bytes.Equal(before, original) {
		t.Fatal("source not preserved", err)
	}
}

func TestInvalidSavedSourcesNeverQueue(t *testing.T) {
	store, bots, m := open(t, t.TempDir())
	defer store.Close()
	defer m.Close()
	random, err := bots.SaveCopy(library.SaveRequest{Command: library.Command{CommandID: "saved-random"}, SourceID: library.RandomID, Name: "Random copy"})
	if err != nil {
		t.Fatal(err)
	}
	td, err := training.NewTD(training.DefaultTDConfig())
	if err != nil {
		t.Fatal(err)
	}
	neural, err := bots.SaveNeural("Neural source", td.Parameters[:], "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{library.HeuristicID, library.RandomID, "missing", random.ID, neural.ID} {
		_, err = m.Start(context.Background(), jobs.StartRequest{Command: library.Command{CommandID: strings.Repeat("x", i+1)}, Name: "Invalid", Config: config(), SourceBotID: id})
		if err == nil {
			t.Fatal("accepted invalid source", id)
		}
	}
	xs, err := m.List(jobs.Training)
	if err != nil || len(xs) != 0 {
		t.Fatal("invalid sources queued", err)
	}
}
