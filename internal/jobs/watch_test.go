package jobs_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/replay"
)

func TestTrainingWatchRestoresActualGameWithoutChangingWork(t *testing.T) {
	dir := t.TempDir()
	store, _, m := open(t, dir)
	c := config()
	c.Generations = 1
	c.Workers = 2
	x, err := m.Start(context.Background(), jobs.StartRequest{Command: library.Command{CommandID: "watch-run"}, Name: "Watch training", Config: c})
	if err != nil {
		t.Fatal(err)
	}
	x = wait(t, m, x.ID)
	before := x.Counters
	viewed, err := m.Watch(x.ID)
	if err != nil {
		t.Fatal(err)
	}
	if x.WatchedGame == nil || viewed.Key != x.WatchedGame.Key || len(viewed.Positions) != len(viewed.Replay.Events)+1 {
		t.Fatal("missing authoritative history")
	}
	gen, err := m.Generation(x.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	score := gen.Scores[viewed.Index]
	if viewed.Replay.Seed != score.Seed || len(viewed.Replay.Events) != score.Turns || *viewed.Replay.Outcome != *score.Outcome || viewed.CandidateSide != score.Side || viewed.OpponentID != score.OpponentID {
		t.Fatal("watch game was not evaluated by the trainer")
	}
	final, err := replay.Play(viewed.Replay)
	if err != nil || final != viewed.Positions[len(viewed.Positions)-1] {
		t.Fatal("viewer board differs from the actual game", err)
	}
	viewed.Positions[0].Checkers[0][0] = 0
	viewed.Replay.Events[0].Dice[0] = 9
	unchanged, _ := m.Get(x.ID)
	if unchanged.Counters != before || unchanged.Revision != x.Revision || unchanged.Version != x.Version {
		t.Fatal("watching changed training")
	}
	original, err := m.Watch(x.ID)
	if err != nil || original.Replay.Events[0].Dice[0] == 9 {
		t.Fatal("viewer mutated the durable game")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, _, m = open(t, dir)
	restored, err := m.Watch(x.ID)
	if err != nil || !reflect.DeepEqual(restored, original) {
		t.Fatal("watched game lost on restart", err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	// Optional watcher data did not exist in the original M3 format-1 checkpoint.
	path := filepath.Join("runs", x.ID, "checkpoint.json")
	data, err := store.Read(path, 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	var env struct {
		Checksum string          `json:"checksum"`
		Record   json.RawMessage `json:"record"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatal(err)
	}
	var legacy map[string]json.RawMessage
	json.Unmarshal(env.Record, &legacy)
	delete(legacy, "watch_replay")
	delete(legacy, "watched_game")
	env.Record, _ = json.Marshal(legacy)
	h := sha256.Sum256(env.Record)
	env.Checksum = hex.EncodeToString(h[:])
	data, _ = json.Marshal(env)
	if err := store.WriteAtomic(path, data); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, _, m = open(t, dir)
	defer store.Close()
	defer m.Close()
	loaded, err := m.Get(x.ID)
	if err != nil || loaded.Counters != before || loaded.WatchedGame != nil {
		t.Fatal("legacy checkpoint did not load")
	}
	if _, err := m.Watch(x.ID); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatal("invented an unrecorded game")
	}
}
