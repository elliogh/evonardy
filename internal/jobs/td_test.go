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

	"evonardy/internal/agent"
	"evonardy/internal/game"
	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/neural"
	"evonardy/internal/replay"
	"evonardy/internal/training"
)

func TestNeuralJobsStopResumeAndFrozenPublication(t *testing.T) {
	for _, lambda := range []float64{0, .7} {
		t.Run(fmt.Sprint(lambda), func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			store, bots, m := open(t, dir)
			c := training.DefaultTDConfig()
			c.Games, c.MaxTurns, c.Lambda = 16, 8, lambda
			req := jobs.StartRequest{Command: library.Command{CommandID: "td-resume"}, Name: "Neural resume", Algorithm: c.Algorithm(), TDConfig: &c}
			x, err := m.Start(ctx, req)
			if err != nil {
				t.Fatal(err)
			}
			repeated, err := m.Start(ctx, req)
			if err != nil || repeated.ID != x.ID {
				t.Fatal("neural start was not idempotent")
			}
			deadline := time.Now().Add(10 * time.Second)
			for x.Generation < 1 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
				x, err = m.Get(x.ID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if x.Generation < 1 || x.State != jobs.Running || x.NeuralCandidate == nil {
				t.Fatal("no neural boundary while training")
			}
			watch, err := m.Watch(x.ID)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := replay.Play(watch.Replay); err != nil {
				t.Fatal(err)
			}
			early, err := m.Save(ctx, x.ID, jobs.SaveRequest{Command: library.Command{CommandID: "early-neural", ExpectedVersion: x.Version}, Generation: x.Generation, CandidateID: x.NeuralCandidate.ID, Name: "Early neural snapshot"})
			if err != nil || early.Kind != neural.Version {
				t.Fatal(err)
			}
			frozen, err := bots.Freeze(early.ID)
			if err != nil || frozen.Kind != "neural" {
				t.Fatal("missing frozen neural model")
			}
			x, _ = m.Get(x.ID)
			stop := library.Command{CommandID: "stop-neural", ExpectedVersion: x.Version}
			if _, err := m.Stop(ctx, x.ID, stop); err != nil {
				t.Fatal(err)
			}
			stopped := wait(t, m, x.ID)
			if stopped.State != jobs.Stopped || !stopped.CanResume || stopped.Counters.Updates != stopped.Counters.Decisions {
				t.Fatalf("missing safe neural checkpoint: %+v", stopped)
			}
			if repeated, err := m.Stop(ctx, x.ID, stop); err != nil || repeated.Revision != stopped.Revision {
				t.Fatal("duplicate neural stop changed work")
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
				t.Fatalf("neural boundary changed after restart: %v", err)
			}
			if _, err := m.Resume(ctx, x.ID, library.Command{CommandID: "resume-neural", ExpectedVersion: restored.Version}); err != nil {
				t.Fatal(err)
			}
			done := wait(t, m, x.ID)
			if done.State != jobs.Completed || done.Counters.Games != 16 || done.Counters.TruncatedGames != 16 || done.NeuralCandidate == nil {
				t.Fatalf("incomplete bounded neural run: %+v", done)
			}
			fullReq := req
			fullReq.CommandID = "td-continuous"
			full, err := m.Start(ctx, fullReq)
			if err != nil {
				t.Fatal(err)
			}
			full = wait(t, m, full.ID)
			if full.State != jobs.Completed || done.Counters != full.Counters || !reflect.DeepEqual(done.TDHistory, full.TDHistory) {
				t.Fatal("neural resume changed history/counters")
			}
			var finalPolicies [2]agent.Policy
			for i, run := range []jobs.Snapshot{done, full} {
				card, err := m.Save(ctx, run.ID, jobs.SaveRequest{Command: library.Command{CommandID: fmt.Sprintf("final-%d", i), ExpectedVersion: run.Version}, Generation: run.Generation, CandidateID: run.NeuralCandidate.ID, Name: "Final neural snapshot"})
				if err != nil {
					t.Fatal(err)
				}
				policy, err := bots.Freeze(card.ID)
				if err != nil {
					t.Fatal(err)
				}
				finalPolicies[i] = policy
			}
			if finalPolicies[0] != finalPolicies[1] {
				t.Fatal("neural resume changed weights/inference identity")
			}
			stillFrozen, err := bots.Freeze(early.ID)
			if err != nil || stillFrozen != frozen {
				t.Fatal("continued training changed early publication")
			}
			p := game.Initial(game.White)
			actions, err := game.LegalActions(p, game.Dice{3, 5})
			if err != nil {
				t.Fatal(err)
			}
			first, err := frozen.New(nil).Choose(ctx, p, game.Dice{3, 5}, actions)
			if err != nil {
				t.Fatal(err)
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			bot, err := bots.Agent(early.ID, nil)
			if err != nil {
				t.Fatal(err)
			}
			second, err := bot.Choose(ctx, p, game.Dice{3, 5}, actions)
			if err != nil || second != first {
				t.Fatal("inference depended on the trainer")
			}
		})
	}
}

func TestNeuralCrashRecoveryReusesExactArchivedBoundaries(t *testing.T) {
	dir := t.TempDir()
	store, _, m := open(t, dir)
	c := training.DefaultTDLambdaConfig()
	c.Games, c.MaxTurns = 12, 8
	request := jobs.StartRequest{Command: library.Command{CommandID: "td-crash"}, Name: "Neural crash", TDConfig: &c}
	x, err := m.Start(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("runs", x.ID, "checkpoint.json")
	var checkpoint []byte
	var committed jobs.Snapshot
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := store.Read(path, replay.MaxBytes)
		if err != nil {
			t.Fatal(err)
		}
		var env struct {
			Record json.RawMessage `json:"record"`
		}
		if err := json.Unmarshal(data, &env); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(env.Record, &committed); err != nil {
			t.Fatal(err)
		}
		if committed.Generation > 0 && committed.State == jobs.Running {
			checkpoint = data
			break
		}
		time.Sleep(time.Millisecond)
	}
	if checkpoint == nil {
		t.Fatal("no committed neural running prefix")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteAtomic(path, checkpoint); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, bots, m := open(t, dir)
	defer store.Close()
	defer m.Close()
	interrupted, err := m.Get(x.ID)
	if err != nil || interrupted.State != jobs.Interrupted || interrupted.Counters != committed.Counters {
		t.Fatal("crash recovery lost the committed neural prefix")
	}
	if _, err := m.Resume(context.Background(), x.ID, library.Command{CommandID: "recover-td", ExpectedVersion: interrupted.Version}); err != nil {
		t.Fatal(err)
	}
	done := wait(t, m, x.ID)
	if done.State != jobs.Completed || done.Counters.Games != 12 {
		t.Fatal("recovery did not complete neural run")
	}
	card, err := m.Save(context.Background(), x.ID, jobs.SaveRequest{Command: library.Command{CommandID: "after-crash", ExpectedVersion: done.Version}, Generation: done.Generation, CandidateID: done.NeuralCandidate.ID, Name: "Recovered neural model"})
	if err != nil {
		t.Fatal(err)
	}
	if policy, err := bots.Freeze(card.ID); err != nil || policy.Kind != "neural" {
		t.Fatal("recovered neural model cannot be loaded")
	}
	fullReq := request
	fullReq.CommandID = "td-crash-continuous"
	full, err := m.Start(context.Background(), fullReq)
	if err != nil {
		t.Fatal(err)
	}
	full = wait(t, m, full.ID)
	if full.State != jobs.Completed || full.Counters != done.Counters || !reflect.DeepEqual(full.TDHistory, done.TDHistory) {
		t.Fatal("crash recovery changed deterministic neural work")
	}
	fullCard, err := m.Save(context.Background(), full.ID, jobs.SaveRequest{Command: library.Command{CommandID: "continuous-crash-model", ExpectedVersion: full.Version}, Generation: full.Generation, CandidateID: full.NeuralCandidate.ID, Name: "Continuous neural model"})
	if err != nil || fullCard.ID != card.ID {
		t.Fatal("crash recovery changed neural weights")
	}
	archivePath := filepath.Join(dir, "runs", x.ID, "generations", "0001.json")
	if err := os.WriteFile(archivePath, []byte(`{"parameters":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Save(context.Background(), x.ID, jobs.SaveRequest{Command: library.Command{CommandID: "corrupt-archive", ExpectedVersion: done.Version + 1}, Generation: 1, CandidateID: "td-game-000001", Name: "Bad archive"}); err == nil {
		t.Fatal("published corrupt neural archive")
	}
}

func TestNeuralJobValidationAndMinimalBoundaryRestart(t *testing.T) {
	dir := t.TempDir()
	store, _, m := open(t, dir)
	c := training.DefaultTDConfig()
	c.Games, c.MaxTurns = 1, 1
	for i, req := range []jobs.StartRequest{
		{Config: training.DefaultConfig(), TDConfig: &c},
		{Algorithm: "unsupported", TDConfig: &c},
		{Algorithm: training.TDLambdaAlgorithm, TDConfig: &c},
		{Algorithm: training.TDAlgorithm},
	} {
		req.CommandID, req.Name = fmt.Sprintf("bad-neural-%d", i), "Invalid neural config"
		if _, err := m.Start(context.Background(), req); err == nil {
			t.Fatal("accepted incompatible neural request")
		}
	}
	c.Games = 10001
	if _, err := m.Start(context.Background(), jobs.StartRequest{Command: library.Command{CommandID: "oversized-neural"}, Name: "Oversized", TDConfig: &c}); err == nil {
		t.Fatal("accepted oversized durable history")
	}
	c.Games = 1
	// Completed single-game runs also test loading a minimal bounded history.
	x, err := m.Start(context.Background(), jobs.StartRequest{Command: library.Command{CommandID: "minimal-neural"}, Name: "Minimal", TDConfig: &c})
	if err != nil {
		t.Fatal(err)
	}
	x = wait(t, m, x.ID)
	if x.State != jobs.Completed || x.Counters.TruncatedGames != 1 {
		t.Fatal("truncation must not fail sequential TD training")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, _, m = open(t, dir)
	defer store.Close()
	defer m.Close()
	if restored, err := m.Get(x.ID); err != nil || !reflect.DeepEqual(x, restored) {
		t.Fatal("minimal neural checkpoint did not survive restart")
	}
}
