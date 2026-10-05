package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"evonardy/internal/features"
	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/neural"
	"evonardy/internal/storage"
	"evonardy/internal/training"
)

func TestCLITrainsFreezesAndEvaluates(t *testing.T) {
	dir := t.TempDir()
	var output bytes.Buffer
	if err := run(context.Background(), []string{"train", "--config", filepath.Join("..", "..", "configs", "ga-linear-smoke.json"), "--data-dir", dir, "--save-name", "CLI candidate"}, &output); err != nil {
		t.Fatal(err)
	}
	var x struct {
		jobs.Snapshot
		Bot library.Card `json:"saved_bot"`
	}
	if err := json.Unmarshal(output.Bytes(), &x); err != nil {
		t.Fatal(err)
	}
	if x.State != jobs.Completed || x.Counters.Games != 32 || x.Bot.ID == "" {
		t.Fatal(output.String())
	}
	output.Reset()
	if err := run(context.Background(), []string{"evaluate", "--config", filepath.Join("..", "..", "configs", "evaluation-smoke.json"), "--data-dir", dir, "--bot", x.Bot.ID}, &output); err != nil {
		t.Fatal(err)
	}
	var y jobs.Snapshot
	if err := json.Unmarshal(output.Bytes(), &y); err != nil {
		t.Fatal(err)
	}
	if y.State != jobs.Completed || y.Evaluation.Stats.Games != 8 {
		t.Fatal(output.String())
	}
}

func TestCLINeuralTrainSaveReloadAndEvaluate(t *testing.T) {
	for _, algorithm := range []string{"td0", "td-lambda"} {
		t.Run(algorithm, func(t *testing.T) {
			dir := t.TempDir()
			cfg := training.DefaultTDConfig()
			cfg.Games, cfg.MaxTurns = 2, 8
			if algorithm == "td-lambda" {
				cfg.Lambda = 0.7
			}
			data, err := json.Marshal(cfg)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "td.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if err := run(context.Background(), []string{"train", "--algorithm", algorithm, "--config", path, "--data-dir", dir, "--save-name", "CLI neural snapshot"}, &output); err != nil {
				t.Fatal(err)
			}
			var x struct {
				jobs.Snapshot
				Bot library.Card `json:"saved_bot"`
			}
			if err := json.Unmarshal(output.Bytes(), &x); err != nil {
				t.Fatal(err)
			}
			if x.State != jobs.Completed || x.Algorithm != cfg.Algorithm() || x.Counters.Games != 2 || x.Counters.Updates != 16 || x.Bot.Kind != neural.Version || x.NeuralCandidate == nil || len(x.TDHistory) != 2 {
				t.Fatal(output.String())
			}
			// The second CLI invocation reopens storage and loads the frozen package.
			output.Reset()
			if err := run(context.Background(), []string{"evaluate", "--config", filepath.Join("..", "..", "configs", "evaluation-smoke.json"), "--data-dir", dir, "--bot", x.Bot.ID}, &output); err != nil {
				t.Fatal(err)
			}
			var result jobs.Snapshot
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.State != jobs.Completed || result.Evaluation.Stats == nil || result.Evaluation.Stats.Games != 8 || result.Counters.Updates != 0 {
				t.Fatal(output.String())
			}
		})
	}
}

func TestCLITrainFromSavedLinearModel(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	weights := features.DefaultWeights
	weights[7] = .95
	source, err := library.New(store).SaveLinear("CLI source", weights[:], "fixture", "")
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	var output bytes.Buffer
	args := []string{"train", "--config", filepath.Join("..", "..", "configs", "ga-linear-smoke.json"), "--data-dir", dir, "--source-bot", source.ID, "--save-name", "CLI descendant"}
	if err = run(context.Background(), args, &output); err != nil {
		t.Fatal(err)
	}
	var x jobs.Snapshot
	if err = json.Unmarshal(output.Bytes(), &x); err != nil {
		t.Fatal(err)
	}
	if x.State != jobs.Completed || x.SourceBotID != source.ID || x.Counters.Games != 48 || x.Algorithm != training.FromModelAlgorithm {
		t.Fatal("CLI lost source or budget", output.String())
	}
	store, err = storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	frozen, err := library.New(store).Freeze(source.ID)
	if err != nil || frozen.Weights != weights {
		t.Fatal("CLI changed source", err)
	}
}
