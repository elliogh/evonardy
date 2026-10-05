package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"evonardy/internal/arena"
	"evonardy/internal/game"
	"evonardy/internal/replay"
	"evonardy/internal/storage"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "evonardy:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprintln(out, "EvoNardy\n  serve [--addr 127.0.0.1:8080] [--data-dir ./data] [--web-dir web/dist]\n  train --config file [--algorithm ga-linear|ga-mlp|td0|td-lambda|hybrid] [--name name] [--data-dir ./data] [--save-name name]\n  resume --run run-id [--data-dir ./data]\n  evaluate --bot bot-id [--config file] [--data-dir ./data]\n  experiment --config file [--name name] [--data-dir ./data]\n  experiment-resume --experiment id [--data-dir ./data]\n  bots list|save [--source bot-id] [--name name] [--data-dir ./data]\n  simulate [--config file] [--seed N] [--games N] [--workers N] [--max-turns N] [--white random|heuristic] [--black random|heuristic] [--data-dir ./data]\n  replay --file path.json | --dir replay-directory\nDevelopment: make dev. SIGINT checkpoints an active training/evaluation job.")
		return err
	}
	switch args[0] {
	case "serve":
		return serveCommand(ctx, args[1:], out)
	case "bots":
		return botsCommand(ctx, args[1:], out)
	case "train", "resume", "evaluate":
		return jobCommand(ctx, args[0], args[1:], out)
	case "experiment", "experiment-resume":
		return experimentCommand(ctx, args[0], args[1:], out)
	case "simulate":
		return simulateCommand(ctx, args[1:], out)
	case "replay":
		return replayCommand(ctx, args[1:], out)
	default:
		return fmt.Errorf("unknown or unavailable command %q; use --help", args[0])
	}
}

type summary struct {
	Version         int          `json:"version"`
	RunID           string       `json:"run_id"`
	Ruleset         string       `json:"ruleset"`
	Config          arena.Config `json:"config"`
	Completed       int          `json:"completed"`
	Truncated       int          `json:"truncated"`
	WhiteWins       int          `json:"white_wins"`
	BlackWins       int          `json:"black_wins"`
	Mars            int          `json:"mars"`
	TotalTurns      int          `json:"total_turns"`
	WhiteWinRate    *float64     `json:"white_win_rate"`
	MeanWhiteScore  *float64     `json:"mean_white_score"`
	WallSeconds     float64      `json:"wall_seconds"`
	ReplayDirectory string       `json:"replay_directory"`
}

func simulateCommand(ctx context.Context, args []string, out io.Writer) error {
	cfg := arena.Config{Seed: 42, Games: 4, Workers: 2, MaxTurns: 1200, White: "heuristic", Black: "random"}
	flags := flag.NewFlagSet("simulate", flag.ContinueOnError)
	flags.SetOutput(out)
	config := flags.String("config", "", "JSON simulation config")
	dir := flags.String("data-dir", "./data", "exclusive artifact directory")
	flags.Uint64Var(&cfg.Seed, "seed", cfg.Seed, "environment seed")
	flags.IntVar(&cfg.Games, "games", cfg.Games, "number of games")
	flags.IntVar(&cfg.Workers, "workers", cfg.Workers, "bounded workers")
	flags.IntVar(&cfg.MaxTurns, "max-turns", cfg.MaxTurns, "truncation guard")
	flags.StringVar(&cfg.White, "white", cfg.White, "white baseline")
	flags.StringVar(&cfg.Black, "black", cfg.Black, "black baseline")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected simulation arguments")
	}
	if *config != "" {
		var loaded arena.Config
		if err := readConfig(*config, &loaded); err != nil {
			return err
		}
		flags.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "seed":
				loaded.Seed = cfg.Seed
			case "games":
				loaded.Games = cfg.Games
			case "workers":
				loaded.Workers = cfg.Workers
			case "max-turns":
				loaded.MaxTurns = cfg.MaxTurns
			case "white":
				loaded.White = cfg.White
			case "black":
				loaded.Black = cfg.Black
			}
		})
		cfg = loaded
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	store, err := storage.Open(*dir)
	if err != nil {
		return err
	}
	defer store.Close()
	start := time.Now()
	records, err := arena.Run(ctx, cfg)
	if err != nil {
		return err
	}
	wall := time.Since(start).Seconds()
	var token [8]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	id := time.Now().UTC().Format("20060102T150405") + "-" + hex.EncodeToString(token[:])
	relativeDir := filepath.Join("replays", id)
	s := summary{Version: 1, RunID: id, Ruleset: game.Ruleset, Config: cfg, WallSeconds: wall, ReplayDirectory: filepath.Join(*dir, relativeDir)}
	points := 0
	for _, r := range records {
		if err := ctx.Err(); err != nil {
			return err
		}
		var data bytes.Buffer
		if err := replay.Encode(&data, r); err != nil {
			return err
		}
		if err := store.WriteNew(filepath.Join(relativeDir, fmt.Sprintf("game-%04d.json", r.GameID)), data.Bytes()); err != nil {
			return err
		}
		s.TotalTurns += len(r.Events)
		if r.Status == replay.Truncated {
			s.Truncated++
			continue
		}
		s.Completed++
		if r.Outcome.Mars {
			s.Mars++
		}
		if r.Outcome.Winner == game.White {
			s.WhiteWins++
			points += r.Outcome.Points
		} else {
			s.BlackWins++
			points -= r.Outcome.Points
		}
	}
	if s.Completed > 0 {
		rate, mean := float64(s.WhiteWins)/float64(s.Completed), float64(points)/float64(s.Completed)
		s.WhiteWinRate = &rate
		s.MeanWhiteScore = &mean
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := store.WriteNew(filepath.Join("runs", id, "summary.json"), append(data, '\n')); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}

func readConfig(path string, value any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 65537))
	if err != nil {
		return err
	}
	if len(data) > 65536 {
		return fmt.Errorf("config exceeds 64 KiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing config data")
	}
	return nil
}

func loadReplay(path string) (replay.Record, error) {
	f, err := os.Open(path)
	if err != nil {
		return replay.Record{}, err
	}
	defer f.Close()
	return replay.Decode(f)
}

func replayCommand(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("replay", flag.ContinueOnError)
	flags.SetOutput(out)
	file := flags.String("file", "", "one replay")
	dir := flags.String("dir", "", "verify all JSON replays recursively")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 || (*file == "") == (*dir == "") {
		return fmt.Errorf("specify exactly one of --file or --dir")
	}
	e := json.NewEncoder(out)
	e.SetIndent("", "  ")
	if *file != "" {
		if err := ctx.Err(); err != nil {
			return err
		}
		r, err := loadReplay(*file)
		if err != nil {
			return err
		}
		final, err := replay.Play(r)
		if err != nil {
			return err
		}
		return e.Encode(struct {
			Status  string        `json:"status"`
			Turns   int           `json:"turns"`
			Outcome *game.Outcome `json:"outcome"`
			Final   game.Position `json:"final"`
			Hash    string        `json:"hash"`
		}{r.Status, len(r.Events), r.Outcome, final, r.FinalHash})
	}
	verified, completed, truncated := 0, 0, 0
	err := filepath.WalkDir(*dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.Type().IsRegular() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil
		}
		if verified >= 10000 {
			return fmt.Errorf("replay batch exceeds 10000 files")
		}
		r, err := loadReplay(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		verified++
		if r.Status == replay.Completed {
			completed++
		} else {
			truncated++
		}
		return nil
	})
	if err != nil {
		return err
	}
	if verified == 0 {
		return fmt.Errorf("no replay files found")
	}
	return e.Encode(struct {
		Verified  int `json:"verified"`
		Completed int `json:"completed"`
		Truncated int `json:"truncated"`
	}{verified, completed, truncated})
}
