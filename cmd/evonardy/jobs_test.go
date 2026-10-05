package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"evonardy/internal/jobs"
	"evonardy/internal/library"
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
