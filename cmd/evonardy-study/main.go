// evonardy-study runs the local strength protocol outside normal verification.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"evonardy/internal/library"
	"evonardy/internal/storage"
	"evonardy/internal/study"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "evonardy-study:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	f := flag.NewFlagSet("evonardy-study", flag.ContinueOnError)
	dir := f.String("data-dir", "", "isolated study directory (required)")
	incumbentDir := f.String("incumbent-dir", "", "archived immutable model package (required for a new study)")
	config := f.String("config", "", "exact study config JSON; defaults to the approved five-method protocol")
	resume := f.Bool("resume", false, "resume the saved protocol with its original deadline")
	printConfig := f.Bool("print-config", false, "print the default protocol and exit")
	installTo := f.String("install-to", "", "install selected completed results into an exclusively owned application directory")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	c := study.DefaultConfig()
	if *printConfig {
		return json.NewEncoder(os.Stdout).Encode(c)
	}
	if *dir == "" || (!*resume && *incumbentDir == "") || (*resume && *config != "") {
		return fmt.Errorf("new study requires --data-dir and --incumbent-dir; resume uses the saved config")
	}
	if *config != "" {
		b, err := os.ReadFile(*config)
		if err != nil {
			return err
		}
		if err = library.DecodeJSON(b, &c); err != nil {
			return err
		}
	}
	if err := c.Validate(); err != nil {
		return err
	}
	s, err := storage.Open(*dir)
	if err != nil {
		return err
	}
	defer s.Close()
	id := ""
	if *incumbentDir != "" {
		files := map[string][]byte{}
		for _, name := range []string{"manifest.json", "model.json", "metadata.json"} {
			b, e := os.ReadFile(filepath.Join(*incumbentDir, name))
			if e != nil {
				return e
			}
			files[name] = b
		}
		var m library.Manifest
		if err = library.DecodeJSON(files["manifest.json"], &m); err != nil {
			return err
		}
		if b, e := hex.DecodeString(m.ID); e != nil || len(b) != 32 || m.ID != hex.EncodeToString(b) {
			return fmt.Errorf("invalid package identity")
		}
		id = m.ID
		if err = s.PublishNew(filepath.Join("bots", id), files); err != nil && !os.IsExist(err) {
			return err
		}
		if _, err = library.New(s).Freeze(id); err != nil {
			return err
		}
	}
	x, err := study.Execute(ctx, s, id, c, *resume, os.Stdout)
	if x.ID != "" {
		fmt.Printf("Study %s: %s. Report: %s\n", x.ID, x.Status, filepath.Join(*dir, "study", "report.md"))
	}
	if err == nil && *installTo != "" {
		destination, e := storage.Open(*installTo)
		if e != nil {
			return e
		}
		defer destination.Close()
		if e = study.Install(s, destination, x); e != nil {
			return e
		}
		fmt.Println("Selected packages installed in", *installTo)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}
