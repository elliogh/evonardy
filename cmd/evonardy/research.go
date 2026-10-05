package main

import (
	"context"
	"encoding/json"
	"errors"
	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/research"
	"evonardy/internal/storage"
	"flag"
	"fmt"
	"io"
)

func experimentCommand(ctx context.Context, kind string, args []string, out io.Writer) error {
	flags := flag.NewFlagSet(kind, flag.ContinueOnError)
	flags.SetOutput(out)
	dir := flags.String("data-dir", "./data", "exclusive application directory")
	file := flags.String("config", "", "research configuration JSON")
	name := flags.String("name", "Research experiment", "display name")
	experiment := flags.String("experiment", "", "interrupted or stopped experiment ID")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || (kind == "experiment" && (*file == "" || *experiment != "")) || (kind == "experiment-resume" && (*experiment == "" || *file != "")) {
		return fmt.Errorf("experiment requires --config; experiment-resume requires --experiment and uses saved config")
	}
	cfg := research.DefaultConfig()
	if kind == "experiment" {
		if err := readConfig(*file, &cfg); err != nil {
			return err
		}
		if err := cfg.Validate(); err != nil {
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
	m, err := research.New(store, bots)
	if err != nil {
		return err
	}
	defer m.Close()
	id, err := commandID()
	if err != nil {
		return err
	}
	cmd := library.Command{CommandID: id}
	var x research.Snapshot
	if kind == "experiment" {
		x, err = m.Start(ctx, research.StartRequest{Command: cmd, Name: *name, Config: cfg})
	} else {
		x, err = m.Get(*experiment)
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
		if err = m.Close(); err != nil {
			return err
		}
		result, err = m.Get(x.ID)
	}
	if err != nil {
		return err
	}
	e := json.NewEncoder(out)
	e.SetIndent("", "  ")
	if err = e.Encode(result); err != nil {
		return err
	}
	if result.State == jobs.Failed {
		return fmt.Errorf("experiment %s failed: %s", result.ID, result.Error)
	}
	return nil
}
