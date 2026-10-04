package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"evonardy/internal/game"
	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/random"
	"evonardy/internal/storage"
	"evonardy/internal/training"
)

func open(t *testing.T, dir string) (*storage.Store, *library.Library, *jobs.Manager) {
	t.Helper()
	store, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	bots := library.New(store)
	m, err := jobs.New(store, bots)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	return store, bots, m
}
func wait(t *testing.T, m *jobs.Manager, id string) jobs.Snapshot {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	x, err := m.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return x
}
func config() training.Config {
	c := training.DefaultConfig()
	c.Population = 4
	c.Generations = 3
	c.PairsPerOpponent = 1
	c.Workers = 1
	return c
}
func TestStopReopenResumeAndPublishKeepActualModelsAndWork(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, bots, m := open(t, dir)
	request := jobs.StartRequest{Command: library.Command{CommandID: "resumable"}, Name: "Resume test", Config: config()}
	x, err := m.Start(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for x.Generation < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond * 5)
		x, err = m.Get(x.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if x.Generation == 0 || x.State != jobs.Running {
		t.Fatal("no evaluated generation while training")
	}
	early, err := m.Save(ctx, x.ID, jobs.SaveRequest{Command: library.Command{CommandID: "publish-early", ExpectedVersion: x.Version}, Generation: x.Generation, CandidateID: x.Candidates[1].ID, Name: "Early frozen candidate"})
	if err != nil {
		t.Fatal(err)
	}
	frozenEarly, err := bots.Freeze(early.ID)
	if err != nil {
		t.Fatal(err)
	}
	x, err = m.Get(x.ID)
	if err != nil {
		t.Fatal(err)
	}
	stop := library.Command{CommandID: "stop", ExpectedVersion: x.Version}
	if _, err := m.Stop(ctx, x.ID, stop); err != nil {
		t.Fatal(err)
	}
	stopped := wait(t, m, x.ID)
	if stopped.State != jobs.Stopped || !stopped.CanResume {
		t.Fatal("missing resumable checkpoint")
	}
	same, err := m.Stop(ctx, x.ID, stop)
	if err != nil || same.Revision != stopped.Revision {
		t.Fatal("duplicate stop changed state")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, bots, m = open(t, dir)
	defer store.Close()
	defer m.Close()
	restored, err := m.Get(x.ID)
	if err != nil || !reflect.DeepEqual(stopped, restored) {
		t.Fatal("checkpoint changed after restart")
	}
	if _, err := m.Resume(ctx, x.ID, library.Command{CommandID: "resume", ExpectedVersion: restored.Version}); err != nil {
		t.Fatal(err)
	}
	done := wait(t, m, x.ID)
	if done.State != jobs.Completed || done.Generation != 3 || done.Counters.Games != 48 {
		t.Fatalf("run: %+v", done)
	}
	stillFrozen, err := bots.Freeze(early.ID)
	if err != nil || stillFrozen != frozenEarly {
		t.Fatal("resume overwrote an early published model")
	}
	fullReq := request
	fullReq.CommandID = "continuous"
	y, err := m.Start(ctx, fullReq)
	if err != nil {
		t.Fatal(err)
	}
	full := wait(t, m, y.ID)
	if !reflect.DeepEqual(done.Candidates, full.Candidates) || !reflect.DeepEqual(done.History, full.History) || done.Counters != full.Counters {
		t.Fatal("manager resume changed deterministic results")
	}
	save := jobs.SaveRequest{Command: library.Command{CommandID: "publish", ExpectedVersion: done.Version}, Generation: done.Generation, CandidateID: done.Candidates[1].ID, Name: "Trained candidate"}
	card, err := m.Save(ctx, done.ID, save)
	if err != nil {
		t.Fatal(err)
	}
	before, err := bots.Freeze(card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.Weights != done.Candidates[1].Weights {
		t.Fatal("saved coefficients differ from the evaluated candidate")
	}
	repeated, err := m.Save(ctx, done.ID, save)
	if err != nil || repeated.ID != card.ID {
		t.Fatal("save was not idempotent")
	}
	evaluation := jobs.EvaluateRequest{Command: library.Command{CommandID: "evaluate"}, BotID: card.ID, Config: jobs.EvaluationConfig{Seed: 2026, Pairs: 1, Workers: 2, MaxTurns: 1200, OpponentIDs: []string{library.HeuristicID, library.RandomID}}}
	e, err := m.Evaluate(ctx, evaluation)
	if err != nil {
		t.Fatal(err)
	}
	e = wait(t, m, e.ID)
	if e.State != jobs.Completed || e.Evaluation == nil || e.Evaluation.Stats == nil || e.Evaluation.Stats.Games != 4 || e.Counters.Games != 4 {
		t.Fatal("missing measured independent evaluation")
	}
	after, err := bots.Freeze(card.ID)
	if err != nil || after != before {
		t.Fatal("evaluation modified the published model")
	}
	// A published model performs ordinary inference without a manager.
	p := game.Initial(game.White)
	actions, err := game.LegalActions(p, game.Dice{3, 5})
	if err != nil {
		t.Fatal(err)
	}
	first, err := before.New(random.New(42, "saved-policy", 0)).Choose(ctx, p, game.Dice{3, 5}, actions)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := bots.Freeze(card.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := reloaded.New(random.New(42, "saved-policy", 0)).Choose(ctx, p, game.Dice{3, 5}, actions)
	if err != nil || first != second {
		t.Fatal("inference changed without the trainer")
	}
	if _, err := m.Stop(ctx, done.ID, library.Command{CommandID: "stale", ExpectedVersion: 0}); !errors.Is(err, jobs.ErrConflict) {
		t.Fatal("stale control command accepted")
	}
}
func TestFailedTruncatedRunNeverPublishesAFitness(t *testing.T) {
	store, _, m := open(t, t.TempDir())
	defer store.Close()
	defer m.Close()
	c := config()
	c.MaxTurns = 1
	x, err := m.Start(context.Background(), jobs.StartRequest{Command: library.Command{CommandID: "truncated"}, Name: "Truncated", Config: c})
	if err != nil {
		t.Fatal(err)
	}
	x = wait(t, m, x.ID)
	if x.State != jobs.Failed || x.Generation != 0 || len(x.Candidates) != 0 || len(x.History) != 0 || x.Error == "" {
		t.Fatal("truncated generation selected or received fitness")
	}
}

func TestCrashRecoveryPreservesCommittedPrefixAndRejectsCorruption(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, _, m := open(t, dir)
	c := config()
	c.Generations = 2
	x, err := m.Start(ctx, jobs.StartRequest{Command: library.Command{CommandID: "crash"}, Name: "Crash recovery", Config: c})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("runs", x.ID, "checkpoint.json")
	var checkpoint []byte
	var committed jobs.Snapshot
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := store.Read(path, 16<<20)
		if err != nil {
			t.Fatal(err)
		}
		var env struct {
			Record json.RawMessage `json:"record"`
		}
		json.Unmarshal(data, &env)
		json.Unmarshal(env.Record, &committed)
		if committed.Counters.Games > 0 && committed.State == jobs.Running {
			checkpoint = data
			break
		}
		time.Sleep(time.Millisecond * 5)
	}
	if checkpoint == nil {
		t.Fatal("no committed running checkpoint")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	// Restore the exact last atomic file as if the process died before its next commit.
	if err := store.WriteAtomic(path, checkpoint); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, _, m = open(t, dir)
	interrupted, err := m.Get(x.ID)
	if err != nil || interrupted.State != jobs.Interrupted || interrupted.Counters != committed.Counters {
		t.Fatal(interrupted, err)
	}
	if _, err := m.Resume(ctx, x.ID, library.Command{CommandID: "recover", ExpectedVersion: interrupted.Version}); err != nil {
		t.Fatal(err)
	}
	recovered := wait(t, m, x.ID)
	if recovered.State != jobs.Completed || recovered.Counters.Games != 32 {
		t.Fatal(recovered)
	}
	complete, err := m.Start(ctx, jobs.StartRequest{Command: library.Command{CommandID: "complete"}, Name: "Continuous", Config: c})
	if err != nil {
		t.Fatal(err)
	}
	complete = wait(t, m, complete.ID)
	if !reflect.DeepEqual(recovered.Candidates, complete.Candidates) || !reflect.DeepEqual(recovered.History, complete.History) || recovered.Counters != complete.Counters {
		t.Fatal("crash recovery changed the run")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	corrupt := append([]byte{}, checkpoint...)
	for i := range corrupt {
		if corrupt[i] == '4' {
			corrupt[i] = '5'
			break
		}
	}
	if err := store.WriteAtomic(path, corrupt); err != nil {
		t.Fatal(err)
	}
	if manager, err := jobs.New(store, library.New(store)); err == nil {
		manager.Close()
		t.Fatal("corrupted checkpoint accepted")
	}
	store.Close()
}

func TestEvaluationOwnsItsScheduleAndUsesSeparateSeeds(t *testing.T) {
	store, _, m := open(t, t.TempDir())
	defer store.Close()
	defer m.Close()
	cfg := jobs.DefaultEvaluationConfig()
	cfg.Seed = 42
	cfg.Pairs = 1
	x, err := m.Evaluate(context.Background(), jobs.EvaluateRequest{Command: library.Command{CommandID: "frozen-schedule"}, BotID: library.HeuristicID, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	cfg.OpponentIDs[0] = "mutated-caller-input"
	done := wait(t, m, x.ID)
	if done.State != jobs.Completed || done.Evaluation.Config.OpponentIDs[0] != library.HeuristicID {
		t.Fatal("caller mutated schedule")
	}
	actual, err := m.Get(x.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, score := range actual.Evaluation.Scores {
		if score.Seed == training.PairSeed(42, "ga/development", 0, 0) {
			t.Fatal("independent evaluation reused development seed")
		}
	}
	actual.Evaluation.Config.OpponentIDs[0] = "mutated-snapshot"
	again, _ := m.Get(x.ID)
	if again.Evaluation.Config.OpponentIDs[0] != library.HeuristicID {
		t.Fatal("returned snapshot mutated job")
	}
	cfg = jobs.DefaultEvaluationConfig()
	cfg.Seed = 42
	cfg.Pairs = 1
	cfg.Workers = 1
	serial, err := m.Evaluate(context.Background(), jobs.EvaluateRequest{Command: library.Command{CommandID: "serial-eval"}, BotID: library.HeuristicID, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	serial = wait(t, m, serial.ID)
	if !reflect.DeepEqual(serial.Evaluation.Stats, done.Evaluation.Stats) || !reflect.DeepEqual(serial.Evaluation.Scores, done.Evaluation.Scores) || serial.Counters != done.Counters {
		t.Fatal("evaluation worker count changed results")
	}
	cfg.MaxTurns = 1
	truncated, err := m.Evaluate(context.Background(), jobs.EvaluateRequest{Command: library.Command{CommandID: "truncated-eval"}, BotID: library.HeuristicID, Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	truncated = wait(t, m, truncated.ID)
	if truncated.State != jobs.Failed || truncated.Evaluation.Stats != nil || truncated.Counters.TruncatedGames != 1 {
		t.Fatal("truncated evaluation received an aggregate score")
	}
}
