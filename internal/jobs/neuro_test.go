package jobs_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/neural"
	"evonardy/internal/training"
)

func waitPopulation(t *testing.T, m *jobs.Manager, id string) jobs.Snapshot {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	x, err := m.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return x
}
func TestPopulationDurableResumeArchiveAndPublication(t *testing.T) {
	for _, algorithm := range []string{training.GAMLPAlgorithm, training.HybridAlgorithm} {
		t.Run(algorithm, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			store, bots, m := open(t, dir)
			c := training.DefaultGAMLPConfig()
			c.Population = 4
			c.Generations = 1
			c.PairsPerOpponent = 1
			c.Workers = 2
			h := training.DefaultHybridConfig()
			h.Rounds = 2
			h.GamesPerRound = 1
			req := jobs.StartRequest{Command: library.Command{CommandID: "population-resume"}, Name: "Population", Algorithm: algorithm, Config: c}
			if algorithm == training.HybridAlgorithm {
				req.Config = training.Config{}
				req.HybridConfig = &h
			}
			x, err := m.Start(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			again, err := m.Start(ctx, req)
			if err != nil || again.ID != x.ID {
				t.Fatal("start not idempotent", err)
			}
			deadline := time.Now().Add(10 * time.Second)
			for x.Counters.Games == 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
				x, err = m.Get(x.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if x.Counters.Games == 0 || x.State != jobs.Running {
				t.Fatal("no running boundary", x.State)
			}
			// Reading and modifying detached DTOs must not affect learner streams or parameters.
			if _, err := m.Watch(x.ID); err != nil {
				t.Fatal(err)
			}
			if x.PopulationProgress == nil {
				t.Fatal("no population progress")
			}
			x.PopulationProgress.Phase = "changed"
			stop := library.Command{CommandID: "stop-population", ExpectedVersion: x.Version}
			if _, err := m.Stop(ctx, x.ID, stop); err != nil {
				t.Fatal(err)
			}
			stopped := waitPopulation(t, m, x.ID)
			if stopped.State != jobs.Stopped {
				t.Fatal(stopped.State)
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			store.Close()
			store, bots, m = open(t, dir)
			defer store.Close()
			defer m.Close()
			restored, err := m.Get(x.ID)
			if err != nil || !reflect.DeepEqual(restored, stopped) {
				t.Fatal("restart changed boundary", err)
			}
			if _, err := m.Resume(ctx, x.ID, library.Command{CommandID: "resume-population", ExpectedVersion: restored.Version}); err != nil {
				t.Fatal(err)
			}
			done := waitPopulation(t, m, x.ID)
			if done.State != jobs.Completed || len(done.NeuralCandidates) == 0 {
				t.Fatal("population failed", done.Error)
			}
			req.CommandID = "population-continuous"
			full, err := m.Start(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			full = waitPopulation(t, m, full.ID)
			if full.State != jobs.Completed || done.Counters != full.Counters || !reflect.DeepEqual(done.History, full.History) || !reflect.DeepEqual(done.NeuralCandidates, full.NeuralCandidates) {
				t.Fatal("resumed population diverged", full.Error)
			}
			var identities []string
			for i, run := range []jobs.Snapshot{done, full} {
				card, err := m.Save(ctx, run.ID, jobs.SaveRequest{Command: library.Command{CommandID: fmt.Sprintf("publish-%d", i), ExpectedVersion: run.Version}, Generation: run.Generation, CandidateID: run.NeuralCandidates[0].ID, Name: "Neural population"})
				if err != nil || card.Kind != neural.Version {
					t.Fatal("publication failed", err)
				}
				identities = append(identities, card.ID)
			}
			if identities[0] != identities[1] {
				t.Fatal("restored parameters changed")
			}
			if algorithm == training.HybridAlgorithm {
				gen, err := m.Generation(done.ID, 1)
				if err != nil || len(gen.Replacements) != 2 || gen.TrainingCounters.Games != 8 {
					t.Fatal("replacement archive missing", err)
				}
				if done.PopulationProgress.TrainingGames != 16 || done.PopulationProgress.SelectionGames != 64 || done.Counters.Games != 80 || done.Counters.Updates == 0 {
					t.Fatal("discarded work missing", done.Counters)
				}
			}
			// Verify archive integrity before publication and keep previously frozen models playable.
			path := filepath.Join(dir, "runs", done.ID, "generations", fmt.Sprintf("%04d.json", done.Generation))
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var archive map[string]any
			json.Unmarshal(data, &archive)
			archive["number"] = 999
			data, _ = json.Marshal(archive)
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			current, _ := m.Get(done.ID)
			if _, err := m.Save(ctx, done.ID, jobs.SaveRequest{Command: library.Command{CommandID: "corrupt-population", ExpectedVersion: current.Version}, Generation: done.Generation, CandidateID: done.NeuralCandidates[0].ID, Name: "Corrupt"}); err == nil {
				t.Fatal("accepted altered archive")
			}
			frozen, err := bots.Freeze(identities[0])
			if err != nil || frozen.Kind != "neural" {
				t.Fatal("frozen inference unavailable", err)
			}
		})
	}
}
func TestPopulationTruncationAndExclusiveConfigs(t *testing.T) {
	store, _, m := open(t, t.TempDir())
	defer store.Close()
	defer m.Close()
	c := training.DefaultGAMLPConfig()
	c.Population = 4
	c.Generations = 1
	c.PairsPerOpponent = 1
	c.MaxTurns = 2
	req := jobs.StartRequest{Command: library.Command{CommandID: "truncated-neuro"}, Name: "Truncated", Algorithm: training.GAMLPAlgorithm, Config: c}
	x, err := m.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	x = waitPopulation(t, m, x.ID)
	if x.State != jobs.Failed || x.Generation != 0 || len(x.NeuralCandidates) != 0 || x.Counters.TruncatedGames != 2 {
		t.Fatal("selected truncated population", x)
	}
	h := training.DefaultHybridConfig()
	req.CommandID = "mixed-neuro"
	req.HybridConfig = &h
	if _, err := m.Start(context.Background(), req); err == nil {
		t.Fatal("accepted mixed configuration")
	}
}

func TestPopulationArchiveAheadCrashRepair(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(fmt.Sprint(corrupt), func(t *testing.T) {
			dir := t.TempDir()
			store, _, m := open(t, dir)
			c := training.DefaultGAMLPConfig()
			c.Population = 4
			c.Generations = 1
			c.PairsPerOpponent = 1
			x, err := m.Start(context.Background(), jobs.StartRequest{Command: library.Command{CommandID: "archive-ahead"}, Name: "Archive repair", Algorithm: training.GAMLPAlgorithm, Config: c})
			if err != nil {
				t.Fatal(err)
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			store.Close()
			data, err := os.ReadFile(filepath.Join(dir, "runs", x.ID, "checkpoint.json"))
			if err != nil {
				t.Fatal(err)
			}
			var envelope struct {
				Record struct {
					NeuroTraining training.NeuroState `json:"neuro_training"`
				} `json:"record"`
			}
			if err := json.Unmarshal(data, &envelope); err != nil {
				t.Fatal(err)
			}
			s := envelope.Record.NeuroTraining
			var gen *training.NeuroGeneration
			for gen == nil {
				var err error
				s, gen, _, err = training.StepNeuro(context.Background(), s)
				if err != nil {
					t.Fatal(err)
				}
			}
			archive, err := json.Marshal(gen)
			if err != nil {
				t.Fatal(err)
			}
			if corrupt {
				archive = append(archive, ' ')
			}
			path := filepath.Join(dir, "runs", x.ID, "generations", "0001.json")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, archive, 0600); err != nil {
				t.Fatal(err)
			}
			store, _, m = open(t, dir)
			defer store.Close()
			defer m.Close()
			x, err = m.Get(x.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !x.CanResume {
				t.Fatal("interrupted job not resumable", x.State)
			}
			if _, err := m.Resume(context.Background(), x.ID, library.Command{CommandID: "repair-resume", ExpectedVersion: x.Version}); err != nil {
				t.Fatal(err)
			}
			done := waitPopulation(t, m, x.ID)
			if corrupt {
				if done.State != jobs.Failed || done.Generation != 0 {
					t.Fatal("accepted different archive", done.Error)
				}
			} else if done.State != jobs.Completed || done.Counters != s.Counters || !reflect.DeepEqual(done.History, s.History) {
				t.Fatal("archive repair changed work", done.Error)
			}
			actual, err := os.ReadFile(path)
			if err != nil || !reflect.DeepEqual(actual, archive) {
				t.Fatal("existing archive overwritten", err)
			}
		})
	}
}
