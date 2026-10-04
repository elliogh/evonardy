package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"

	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/storage"
	"evonardy/internal/training"
)

func commandID() (string, error) {
	var b [16]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func jobCommand(ctx context.Context, kind string, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(kind, flag.ContinueOnError)
	flags.SetOutput(out)
	dir := flags.String("data-dir", "./data", "exclusive application directory")
	config := flags.String("config", "", "JSON config; required for train")
	name := flags.String("name", "GA-linear experiment", "training display name")
	runID := flags.String("run", "", "interrupted or stopped job ID")
	botID := flags.String("bot", "", "frozen model ID")
	saveName := flags.String("save-name", "", "save the best completed training candidate")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected job arguments")
	}
	if (kind == "train" && *config == "") || (kind == "resume" && *runID == "") || (kind == "evaluate" && *botID == "") {
		return fmt.Errorf("train requires --config; resume requires --run; evaluate requires --bot")
	}
	cfg := training.DefaultConfig()
	ec := jobs.DefaultEvaluationConfig()
	if *config != "" {
		var value any = &cfg
		if kind == "evaluate" {
			value = &ec
		}
		if kind == "resume" {
			return fmt.Errorf("resume uses its saved config")
		}
		if err := readConfig(*config, value); err != nil {
			return err
		}
	}
	if kind == "train" {
		if err := cfg.Validate(); err != nil {
			return err
		}
	}
	if kind == "evaluate" {
		if err := ec.Validate(); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	store, err := storage.Open(*dir)
	if err != nil {
		return err
	}
	defer store.Close()
	bots := library.New(store)
	m, err := jobs.New(store, bots)
	if err != nil {
		return err
	}
	defer m.Close()
	id, err := commandID()
	if err != nil {
		return err
	}
	cmd := library.Command{CommandID: id}
	var x jobs.Snapshot
	switch kind {
	case "train":
		x, err = m.Start(ctx, jobs.StartRequest{Command: cmd, Name: *name, Config: cfg})
	case "evaluate":
		x, err = m.Evaluate(ctx, jobs.EvaluateRequest{Command: cmd, BotID: *botID, Config: ec})
	case "resume":
		x, err = m.Get(*runID)
		if err == nil {
			cmd.ExpectedVersion = x.Version
			x, err = m.Resume(ctx, x.ID, cmd)
		}
	}
	if err != nil {
		return err
	}
	result, err := m.Wait(ctx, x.ID)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if closeErr := m.Close(); closeErr != nil {
			return closeErr
		}
		result, err = m.Get(x.ID)
	}
	if err != nil {
		return err
	}
	var saved *library.Card
	if *saveName != "" && result.Kind == jobs.Training && result.State == jobs.Completed {
		id, err := commandID()
		if err != nil {
			return err
		}
		card, err := m.Save(context.Background(), result.ID, jobs.SaveRequest{Command: library.Command{CommandID: id, ExpectedVersion: result.Version}, Generation: result.Generation, CandidateID: result.Candidates[0].ID, Name: *saveName})
		if err != nil {
			return err
		}
		saved = &card
		result, err = m.Get(result.ID)
		if err != nil {
			return err
		}
	}
	e := json.NewEncoder(out)
	e.SetIndent("", "  ")
	if err := e.Encode(struct {
		jobs.Snapshot
		Bot *library.Card `json:"saved_bot,omitempty"`
	}{result, saved}); err != nil {
		return err
	}
	if result.State == jobs.Failed {
		return fmt.Errorf("job %s failed: %s", result.ID, result.Error)
	}
	return nil
}
