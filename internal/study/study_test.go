package study

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"evonardy/internal/features"
	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/storage"
)

func smallConfig() Config {
	c := DefaultConfig()
	c.Seeds = []uint64{1001, 1002}
	c.TimeLimitSeconds = 600
	c.TrainingCutoffSeconds = 500
	c.DevelopmentPairs, c.RandomPairs, c.TournamentPairs = 1, 1, 1
	c.Bootstrap.Resamples = 100
	c.Confirmation.Pairs = 2
	c.Confirmation.Bootstrap.Resamples = 100
	for i := range c.Methods {
		r := &c.Methods[i].Request
		if r.TDConfig != nil {
			r.TDConfig.Games = 1
		} else if r.HybridConfig != nil {
			r.HybridConfig.Rounds, r.HybridConfig.GamesPerRound, r.HybridConfig.PairsPerOpponent = 1, 1, 1
		} else {
			r.Config.Population, r.Config.Generations, r.Config.PairsPerOpponent = 4, 1, 1
		}
	}
	return c
}

func testStore(t *testing.T) (*storage.Store, string) {
	t.Helper()
	s, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	card, err := library.New(s).SaveLinear("Historical incumbent", features.DefaultWeights[:], "test fixture", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	return s, card.ID
}

func TestFiveMethodsSelectionAndCompletedResume(t *testing.T) {
	s, id := testStore(t)
	cfg := smallConfig()
	x, err := Execute(context.Background(), s, id, cfg, false, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if x.Status != "completed" || len(x.Runs) != 10 || len(x.Methods) != 5 || len(x.Differences) != 10 || !x.SelectionLocked || len(x.Standings) < 2 || len(x.Standings) > 7 || len(x.Matches) != len(x.Standings)*(len(x.Standings)-1)/2 {
		t.Fatalf("incomplete study: %s runs=%d methods=%d standings=%d matches=%d", x.Status, len(x.Runs), len(x.Methods), len(x.Standings), len(x.Matches))
	}
	if x.Verdict != nil && x.Verdict.Status == "confirmed" {
		t.Fatal("tiny study confirmed a replacement")
	}
	if x.SelectedID != id {
		t.Fatal("unconfirmed replacement installed")
	}
	var games uint64
	for _, w := range x.Work {
		games += w.Counters.Games
	}
	wantGames := uint64(208 + 2*len(x.Matches))
	if x.Verdict != nil {
		wantGames += 4
	}
	if games != wantGames {
		t.Fatalf("lost physical work: %d", games)
	}
	for _, e := range x.Evidence {
		if e.JobID == x.ConfirmationID {
			continue
		}
		if x.ConfirmationID != "" {
			for _, f := range x.Evidence {
				if f.JobID == x.ConfirmationID && e.Evaluation.Config.Seed == f.Evaluation.Config.Seed {
					t.Fatal("reused final root")
				}
			}
		}
	}
	before, err := s.Read("study/report.json", 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	resumed, err := Execute(context.Background(), s, "", DefaultConfig(), true, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	after, err := s.Read("study/report.json", 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || resumed.ID != x.ID {
		t.Fatal("completed resume changed evidence")
	}
	if _, err = Execute(context.Background(), s, id, cfg, false, io.Discard); err == nil {
		t.Fatal("overwrote existing study")
	}
	destination, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer destination.Close()
	if err = Install(s, destination, x); err != nil {
		t.Fatal(err)
	}
	if err = Install(s, destination, x); err != nil {
		t.Fatal("installation not idempotent", err)
	}
	if _, err = library.New(destination).Freeze(x.SelectedID); err != nil {
		t.Fatal("missing installed incumbent", err)
	}
	if x.BestNewID != "" && x.BestNewID != x.SelectedID {
		card, err := library.New(destination).Get(x.BestNewID)
		if err != nil || !strings.HasPrefix(card.Name, "Candidate — ") {
			t.Fatal("missing candidate classification", err)
		}
		for _, name := range []string{"manifest.json", "model.json"} {
			path := "bots/" + x.BestNewID + "/" + name
			before, _ := s.Read(path, 1<<20)
			after, _ := destination.Read(path, 1<<20)
			if !bytes.Equal(before, after) {
				t.Fatal("installation changed immutable package")
			}
		}
		card.Card, err = library.New(destination).Rename(card.ID, library.RenameRequest{Command: library.Command{CommandID: "user-rename", ExpectedVersion: card.MetadataVersion}, Name: "My candidate"})
		if err != nil {
			t.Fatal(err)
		}
		if err = Install(s, destination, x); err != nil {
			t.Fatal(err)
		}
		card, _ = library.New(destination).Get(x.BestNewID)
		if card.Name != "My candidate" {
			t.Fatal("reinstallation overwrote user metadata")
		}
	}
	bad := x
	bad.SelectedID = library.RandomID
	bad.Verdict = nil
	if Install(s, destination, bad) == nil {
		t.Fatal("installed unconfirmed replacement")
	}
}

func TestAdmissionAndConfiguredReport(t *testing.T) {
	c := smallConfig()
	c.Seeds = make([]uint64, 20)
	for i := range c.Seeds {
		c.Seeds[i] = uint64(i + 1)
	}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "durable jobs") {
		t.Fatal("accepted protocol above job capacity", err)
	}
	c.Methods = c.Methods[2:3]
	c.DevelopmentPairs, c.Bootstrap.Resamples = 250, 10000
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "pair draws") {
		t.Fatal("accepted over-capacity bootstrap", err)
	}
	c = smallConfig()
	c.Bootstrap.Confidence, c.Confirmation.Bootstrap.Confidence = .9, .8
	report := markdown(State{Config: c})
	if !strings.Contains(report, "90% win interval") || !strings.Contains(report, "80% bootstrap") || strings.Contains(report, "95%") || !strings.Contains(report, "Incomplete or not started: td-lambda seed 1002") {
		t.Fatal("report misrepresented custom protocol or missing seeds")
	}
}

type cancelWriter struct {
	cancel    context.CancelFunc
	completed bool
}

func (w *cancelWriter) Write(p []byte) (int, error) {
	if strings.HasPrefix(string(p), "Completed ") && !w.completed {
		w.completed = true
		w.cancel()
	}
	return len(p), nil
}

func TestInterruptedResumeKeepsCommittedRun(t *testing.T) {
	s, id := testStore(t)
	cfg := smallConfig()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &cancelWriter{cancel: cancel}
	x, err := Execute(ctx, s, id, cfg, false, w)
	if !errors.Is(err, context.Canceled) || x.Status != "interrupted" || len(x.Runs) != 1 {
		t.Fatalf("expected committed interruption: %s %v", x.Status, err)
	}
	first := x.Runs[0]
	y, err := Execute(context.Background(), s, "", DefaultConfig(), true, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if y.Status != "completed" || len(y.Runs) != 10 || y.Runs[0].JobID != first.JobID || y.Runs[0].BotID != first.BotID || !y.StartedAt.Equal(x.StartedAt) {
		t.Fatal("resume repeated or changed committed run")
	}
	seen := map[string]bool{}
	for _, work := range y.Work {
		if seen[work.JobID] {
			t.Fatal("duplicate durable work")
		}
		seen[work.JobID] = true
	}
}

func TestTruncatedWorkNeverSelectsChampion(t *testing.T) {
	s, id := testStore(t)
	cfg := smallConfig()
	cfg.Methods = cfg.Methods[:1]
	cfg.Methods[0].Request.Config.MaxTurns = 1
	x, err := Execute(context.Background(), s, id, cfg, false, io.Discard)
	if err == nil || x.Status != "failed" || x.SelectionLocked || x.Verdict != nil || x.SelectedID != id {
		t.Fatalf("truncated work selected champion: %+v %v", x, err)
	}
	if len(x.Work) != 1 || x.Work[0].Counters.TruncatedGames == 0 {
		t.Fatal("failed work was omitted")
	}
}

func TestChecksumAndFrozenModelValidation(t *testing.T) {
	s, id := testStore(t)
	cfg := smallConfig()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = Execute(ctx, s, id, cfg, false, io.Discard)
	b, err := s.Read("study/checkpoint.json", 16<<20)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]json.RawMessage
	if err = json.Unmarshal(b, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope["sha256"] = json.RawMessage(`"bad"`)
	b, _ = json.Marshal(envelope)
	if err = s.WriteAtomic("study/checkpoint.json", b); err != nil {
		t.Fatal(err)
	}
	if _, err = Execute(context.Background(), s, "", DefaultConfig(), true, io.Discard); err == nil {
		t.Fatal("accepted corrupt journal")
	}
}

func TestDefaultBudgetAndMethodValidation(t *testing.T) {
	c := DefaultConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, m := range c.Methods {
		r := expanded(m, 1001)
		if r.Name == "" || r.CommandID != "" {
			t.Fatal("unexpected job controls")
		}
	}
	sourceConfig := DefaultConfig()
	sourceConfig.Methods[0].Request.SourceBotID = strings.Repeat("a", 64)
	if sourceConfig.Validate() == nil {
		t.Fatal("study accepted an unsupported three-opponent source budget")
	}
	c.Methods[2].Request.Algorithm = jobs.Evaluation
	if c.Validate() == nil {
		t.Fatal("accepted algorithm mismatch")
	}
}

func TestRecoverCompletedJobBeforeStudyCommit(t *testing.T) {
	s, id := testStore(t)
	cfg := smallConfig()
	cfg.Seeds = cfg.Seeds[:1]
	cfg.Methods = cfg.Methods[2:3]
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	x, _ := Execute(ctx, s, id, cfg, false, io.Discard)
	r := runner{store: s, bots: library.New(s), s: x, out: io.Discard}
	m, err := jobs.New(s, r.bots)
	if err != nil {
		t.Fatal(err)
	}
	req := expanded(cfg.Methods[0], cfg.Seeds[0])
	req.Command = r.command("training/td0/1001")
	job, err := m.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	job, err = m.Wait(context.Background(), job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Close(); err != nil {
		t.Fatal(err)
	}
	y, err := Execute(context.Background(), s, "", DefaultConfig(), true, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(y.Runs) != 1 || y.Runs[0].JobID != job.ID {
		t.Fatal("repeated durable training after missing study commit")
	}
	count := 0
	for _, w := range y.Work {
		if w.Kind == jobs.Training {
			count++
		}
	}
	if count != 1 {
		t.Fatal("duplicate training job")
	}
}

func TestCandidateLockAndExpiredDeadline(t *testing.T) {
	s, id := testStore(t)
	cfg := smallConfig()
	cfg.Seeds = cfg.Seeds[:1]
	cfg.Methods = cfg.Methods[2:3]
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	x, _ := Execute(ctx, s, id, cfg, false, io.Discard)
	r := runner{store: s, s: x, out: io.Discard}
	r.s.Phase = "confirmation"
	r.s.CandidateID = library.RandomID
	r.s.SelectionLocked = true
	if err := r.persist(); err != nil {
		t.Fatal(err)
	}
	y, err := Execute(context.Background(), s, "", DefaultConfig(), true, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if y.Verdict == nil || y.Verdict.Status != "candidate" || y.ConfirmationID == "" || y.SelectedID != id || len(y.Evidence) != 1 || len(y.Evidence[0].Evaluation.Scores) != 4 {
		t.Fatal("missing independent candidate confirmation")
	}
	s2, id2 := testStore(t)
	x, _ = Execute(ctx, s2, id2, cfg, false, io.Discard)
	r = runner{store: s2, s: x, out: io.Discard}
	r.s.StartedAt = time.Now().Add(-cfg.duration() - time.Second)
	if err = r.persist(); err != nil {
		t.Fatal(err)
	}
	y, err = Execute(context.Background(), s2, "", DefaultConfig(), true, io.Discard)
	if !errors.Is(err, context.DeadlineExceeded) || y.Status != "time_limit" || y.SelectedID != id2 || len(y.Work) != 0 {
		t.Fatal("resume extended an expired deadline")
	}
}
