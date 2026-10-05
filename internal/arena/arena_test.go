package arena_test

import (
	"context"
	"evonardy/internal/arena"
	"evonardy/internal/replay"
	"reflect"
	"testing"
)

func TestFixedSimulationsAndWorkerOrderReproduce(t *testing.T) {
	cfg := arena.Config{Seed: 42, Games: 4, Workers: 1, MaxTurns: 1200, White: "heuristic", Black: "random"}
	a, err := arena.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Workers = 2
	b, err := arena.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("worker scheduling changed records")
	}
	for _, record := range a {
		if record.Status != replay.Completed {
			t.Fatalf("unexpected truncation: %s", record.Status)
		}
		if _, err := replay.Play(record); err != nil {
			t.Fatal(err)
		}
		if len(record.Opening) == 0 || record.Events[0].Dice != record.Opening[len(record.Opening)-1] {
			t.Fatal("opening dice rerolled")
		}
	}
}

func TestTruncationAndCancellationAreNotResults(t *testing.T) {
	cfg := arena.Config{Seed: 1, Games: 2, Workers: 2, MaxTurns: 1, White: "random", Black: "random"}
	records, err := arena.Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range records {
		if r.Status != replay.Truncated || r.Outcome != nil || len(r.Events) != 1 {
			t.Fatal("truncation turned into sports result")
		}
		if _, err := replay.Play(r); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := arena.Run(ctx, cfg); err == nil {
		t.Fatal("cancellation ignored")
	}
	cfg.White = "fake-bot"
	if _, err := arena.Run(context.Background(), cfg); err == nil {
		t.Fatal("unknown agent accepted")
	}
}

func BenchmarkSimulation(b *testing.B) {
	cfg := arena.Config{Seed: 42, Games: 1, Workers: 1, MaxTurns: 1200, White: "heuristic", Black: "random"}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := arena.Run(context.Background(), cfg); err != nil {
			b.Fatal(err)
		}
	}
}
