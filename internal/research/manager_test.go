package research

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/storage"
	"evonardy/internal/training"
)

func smallConfig() Config {
	c := DefaultConfig()
	c.TrainingSeeds = []uint64{11, 12}
	c.DevelopmentPairs = 1
	c.OpponentIDs = []string{library.HeuristicID}
	c.Confirmation.Pairs = 2
	c.Bootstrap.Resamples = 100
	c.Confirmation.Bootstrap.Resamples = 100
	c.Methods[0].GA.Population = 4
	c.Methods[0].GA.Generations = 1
	c.Methods[0].GA.PairsPerOpponent = 1
	c.Methods[1].TD.Games = 1
	c.Methods[2].Hybrid.Rounds = 1
	c.Methods[2].Hybrid.GamesPerRound = 1
	return c
}
func openTest(t *testing.T, dir string) (*storage.Store, *Manager) {
	t.Helper()
	s, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(s, library.New(s))
	if err != nil {
		t.Fatal(err)
	}
	return s, m
}
func await(t *testing.T, m *Manager, id string) Snapshot {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	x, err := m.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return x
}
func TestExperimentAllMethodsAndReport(t *testing.T) {
	dir := t.TempDir()
	s, m := openTest(t, dir)
	req := StartRequest{Command: library.Command{CommandID: "cohort"}, Name: "Bounded cohort", Config: smallConfig()}
	x, err := m.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	again, err := m.Start(context.Background(), req)
	if err != nil || again.ID != x.ID {
		t.Fatal("creation not idempotent", err)
	}
	req.Name = "Changed"
	if _, err = m.Start(context.Background(), req); err != library.ErrConflict {
		t.Fatal("expected conflict", err)
	}
	x = await(t, m, x.ID)
	if x.State != jobs.Completed {
		t.Fatalf("%s: %s", x.State, x.Error)
	}
	if len(x.Runs) != 6 || len(x.Methods) != 3 || len(x.Differences) != 3 || !x.SelectionLocked || x.Verdict.Status != "candidate" {
		t.Fatalf("incomplete research report %+v", x)
	}
	// Each seed charges all four GA candidates, all eight Hybrid participants,
	// discarded learners and their selection, plus actual development/final games.
	if x.Counters.Games != 130 || x.FinalCounters.Games != 4 {
		t.Fatalf("physical games charged incorrectly: %+v", x.Counters)
	}
	for _, r := range x.Runs {
		if r.BotID == "" || r.Manifest == nil || r.Manifest.Architecture[1] != 32 || r.Counters.Games != r.TrainingGames+r.SelectionGames || r.WallSeconds <= 0 || r.DevelopmentSeconds <= 0 {
			t.Fatalf("missing audit evidence %+v", r)
		}
		if r.Algorithm == "ga-mlp" && r.Counters.Updates != 0 {
			t.Fatal("GA performed TD updates")
		}
		if r.Algorithm == "hybrid" && (r.TrainingGames != 8 || r.SelectionGames != 32 || r.Counters.Updates == 0) {
			t.Fatal("Hybrid selection/training cost lost")
		}
	}
	data, err := m.Report(x.ID)
	if err != nil || hash(data) != x.ReportSHA256 {
		t.Fatal("missing immutable report", err)
	}
	var report Snapshot
	if err = json.Unmarshal(data, &report); err != nil || report.CandidateID != x.CandidateID {
		t.Fatal("invalid report", err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, m = openTest(t, dir)
	defer s.Close()
	defer m.Close()
	reopened, err := m.Get(x.ID)
	if err != nil || !reflect.DeepEqual(x, reopened) {
		t.Fatal("completed experiment changed on reopen", err)
	}
	if _, err = m.Resume(context.Background(), x.ID, library.Command{CommandID: "repeat-final", ExpectedVersion: x.Version}); err != library.ErrConflict {
		t.Fatal("reused final data", err)
	}
	if err = s.WriteAtomic(reportPath(x.ID, x.ReportSHA256), []byte("changed")); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Report(x.ID); err == nil {
		t.Fatal("accepted changed report")
	}
}
func TestStopRestartResumeEqualsUninterrupted(t *testing.T) {
	c := smallConfig()
	td := training.DefaultTDConfig()
	td.Games = 12
	td.MaxTurns = 30
	c.Methods = []Method{{Algorithm: "td0", TD: &td}}
	dir := t.TempDir()
	s, m := openTest(t, dir)
	x, err := m.Start(context.Background(), StartRequest{Command: library.Command{CommandID: "resume"}, Name: "Resume", Config: c})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		x, _ = m.Get(x.ID)
		if x.Active != nil && x.Active.Counters.Games > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	x, err = m.Stop(context.Background(), x.ID, library.Command{CommandID: "stop", ExpectedVersion: x.Version})
	if err != nil {
		t.Fatal(err)
	}
	stopped := await(t, m, x.ID)
	if stopped.State != jobs.Stopped || !stopped.CanResume {
		t.Fatal("not stopped at boundary")
	}
	m.Close()
	s.Close()
	s, m = openTest(t, dir)
	defer s.Close()
	defer m.Close()
	x, _ = m.Get(x.ID)
	x, err = m.Resume(context.Background(), x.ID, library.Command{CommandID: "resume-command", ExpectedVersion: x.Version})
	if err != nil {
		t.Fatal(err)
	}
	resumed := await(t, m, x.ID)
	if resumed.State != jobs.Completed {
		t.Fatal(resumed.Error)
	}
	baselineStore, baseline := openTest(t, t.TempDir())
	defer baselineStore.Close()
	defer baseline.Close()
	normal, err := baseline.Start(context.Background(), StartRequest{Command: library.Command{CommandID: "normal"}, Name: "Normal", Config: c})
	if err != nil {
		t.Fatal(err)
	}
	normal = await(t, baseline, normal.ID)
	if resumed.Counters != normal.Counters || resumed.CandidateID != normal.CandidateID || !reflect.DeepEqual(resumed.FinalScores, normal.FinalScores) || !reflect.DeepEqual(resumed.Methods, normal.Methods) {
		t.Fatal("resume changed work, candidate or random schedules")
	}
	for i, r := range resumed.Runs {
		b := normal.Runs[i]
		if r.Counters != b.Counters || r.BotID != b.BotID || !reflect.DeepEqual(r.Scores, b.Scores) {
			t.Fatal("seed work repeated or changed")
		}
	}
}
func TestTruncationAndCheckpointCorruption(t *testing.T) {
	dir := t.TempDir()
	s, m := openTest(t, dir)
	c := smallConfig()
	c.Methods = c.Methods[:1]
	c.MaxTurns = 1
	x, err := m.Start(context.Background(), StartRequest{Command: library.Command{CommandID: "truncated"}, Name: "Truncated", Config: c})
	if err != nil {
		t.Fatal(err)
	}
	x = await(t, m, x.ID)
	if x.State != jobs.Failed || x.Verdict != nil || x.SelectionLocked || x.Active.DevelopmentCounters.TruncatedGames == 0 || x.Counters.Games == 0 {
		t.Fatal("truncation invented fitness or lost actual work")
	}
	m.Close()
	s.Close()
	data, err := os.ReadFile(dir + "/" + path(x.ID))
	if err != nil {
		t.Fatal(err)
	}
	var e envelope
	json.Unmarshal(data, &e)
	var r record
	json.Unmarshal(e.Record, &r)
	r.Config.FinalSeed = r.Config.DevelopmentSeed
	e.Record, _ = json.Marshal(r)
	e.SHA256 = hash(e.Record)
	data, _ = json.Marshal(e)
	if err = os.WriteFile(dir+"/"+path(x.ID), data, 0600); err != nil {
		t.Fatal(err)
	}
	s, err = storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if bad, err := New(s, library.New(s)); err == nil {
		bad.Close()
		t.Fatal("accepted semantically invalid rehashed checkpoint")
	}
}
func TestConfigBounds(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"duplicate-seed", "phase-root", "algorithm", "budget", "strata", "bootstrap"} {
		t.Run(kind, func(t *testing.T) {
			c := smallConfig()
			switch kind {
			case "duplicate-seed":
				c.TrainingSeeds[1] = c.TrainingSeeds[0]
			case "phase-root":
				c.FinalSeed = c.DevelopmentSeed
			case "algorithm":
				c.Methods[0].Algorithm = "ga-linear"
			case "budget":
				c.Methods[1].TD.Games = 10001
			case "strata":
				c.OpponentIDs = []string{"same", "same"}
			case "bootstrap":
				c.Bootstrap.Resamples = 0
			}
			if err := c.Validate(); err == nil {
				t.Fatal("accepted invalid experiment")
			}
		})
	}
}

func TestHeldoutLockSurvivesInterruption(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Advance explicit safe boundaries without an executor to inject a crash just
	// after the durable selection lock, before any final game has been played.
	m := &Manager{store: s, bots: library.New(s), records: map[string]record{}, wake: make(chan struct{}, 1), changed: make(chan struct{})}
	cfg := smallConfig()
	cfg.Methods = cfg.Methods[1:2]
	x, err := m.Start(context.Background(), StartRequest{Command: library.Command{CommandID: "locked"}, Name: "Locked candidate", Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	r := m.records[x.ID]
	r.State = jobs.Running
	for i := 0; r.Phase != "confirmation" && i < 100; i++ {
		if err = m.step(&r); err != nil {
			t.Fatal(err)
		}
		r.Revision++
		if err = m.persist(r); err != nil {
			t.Fatal(err)
		}
	}
	if !r.SelectionLocked || len(r.FinalScores) != 0 || r.CandidateID == "" {
		t.Fatal("final games preceded selection lock")
	}
	candidate := r.CandidateID
	s.Close()
	s, resumed := openTest(t, dir)
	defer s.Close()
	defer resumed.Close()
	x, _ = resumed.Get(x.ID)
	if x.State != jobs.Interrupted || !x.SelectionLocked || x.CandidateID != candidate {
		t.Fatal("lost lock on interruption")
	}
	x, err = resumed.Resume(context.Background(), x.ID, library.Command{CommandID: "resume-lock", ExpectedVersion: x.Version})
	if err != nil {
		t.Fatal(err)
	}
	x = await(t, resumed, x.ID)
	if x.State != jobs.Completed || x.CandidateID != candidate || len(x.FinalScores) != cfg.Confirmation.Pairs*2 {
		t.Fatal("changed final selection", x.Error)
	}
}

func TestStorageFailureReleasesWaiter(t *testing.T) {
	s, m := openTest(t, t.TempDir())
	cfg := smallConfig()
	cfg.Methods = cfg.Methods[1:2]
	cfg.Methods[0].TD.Games = 12
	x, err := m.Start(context.Background(), StartRequest{Command: library.Command{CommandID: "storage-failure"}, Name: "Storage failure", Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	// An unavailable root must wake CLI waiters rather than leave them waiting for
	// a progress notification that can no longer be persisted.
	s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err = m.Wait(ctx, x.ID); err == nil || err == context.DeadlineExceeded {
		t.Fatal("storage failure did not release waiter", err)
	}
	if err = m.Close(); err == nil {
		t.Fatal("shutdown hid storage failure")
	}
}
