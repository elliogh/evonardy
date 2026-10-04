package main

import (
	"bytes"
	"context"
	"testing"
)

func TestServeRejectsExternalBindingsAndInvalidOrigins(t *testing.T) {
	for _, args := range [][]string{{"serve", "--addr", "0.0.0.0:8080"}, {"serve", "--addr", ":8080"}, {"serve", "--addr", "example.com:8080"}, {"serve", "--dev-origin", "http://external.example:5173"}, {"serve", "--dev-origin", "http://127.0.0.1:5173/path"}} {
		var out bytes.Buffer
		if err := run(context.Background(), args, &out); err == nil {
			t.Fatalf("unsafe configuration accepted: %v", args)
		}
	}
}
func TestBotCLIListsSavedSnapshotsAfterReopen(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	if err := run(context.Background(), []string{"bots", "save", "--name", "CLI snapshot", "--data-dir", dir}, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run(context.Background(), []string{"bots", "list", "--data-dir", dir}, &out); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("CLI snapshot")) || !bytes.Contains(out.Bytes(), []byte("builtin/heuristic-v1")) {
		t.Fatal("saved library missing from CLI")
	}
}
