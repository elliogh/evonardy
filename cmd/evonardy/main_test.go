package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCLIProducesAndVerifiesReplay(t *testing.T) {
	dir := t.TempDir()
	var output bytes.Buffer
	if err := run(context.Background(), []string{"simulate", "--games", "1", "--workers", "1", "--max-turns", "1", "--data-dir", dir}, &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"truncated": 1`)) || !bytes.Contains(output.Bytes(), []byte(`"white_win_rate": null`)) {
		t.Fatalf("wrong summary: %s", output.String())
	}
	files, err := filepath.Glob(filepath.Join(dir, "replays", "*", "*.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("replay publication: %v %v", files, err)
	}
	output.Reset()
	if err := run(context.Background(), []string{"replay", "--file", files[0]}, &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"status": "truncated"`)) {
		t.Fatalf("wrong replay: %s", output.String())
	}
}

func TestCLIConfigOverridesAndInvalidInput(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	if err := os.WriteFile(config, []byte(`{"seed":7,"games":3,"workers":2,"max_turns":1,"white":"random","black":"random"}`), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := run(context.Background(), []string{"simulate", "--config", config, "--games", "1", "--data-dir", filepath.Join(dir, "data")}, &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output.Bytes(), []byte(`"games": 1`)) || !bytes.Contains(output.Bytes(), []byte(`"seed": 7`)) {
		t.Fatalf("override precedence: %s", output.String())
	}
	for _, args := range [][]string{{"train"}, {"simulate", "--games", "0"}, {"simulate", "unexpected"}, {"replay"}, {"replay", "--file", "missing"}} {
		if err := run(context.Background(), args, &output); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	if err := os.WriteFile(config, []byte(`{"seed":1,"unknown":3}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run(context.Background(), []string{"simulate", "--config", config}, &output); err == nil {
		t.Fatal("unknown config field")
	}
}
