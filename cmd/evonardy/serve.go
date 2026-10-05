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
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"evonardy/internal/app"
	"evonardy/internal/httpapi"
	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/storage"
)

func loopbackAddress(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("serve requires a loopback address")
	}
	return nil
}
func serveCommand(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(out)
	addr := flags.String("addr", "127.0.0.1:8080", "loopback listen address")
	dir := flags.String("data-dir", "./data", "exclusive application data directory")
	web := flags.String("web-dir", "web/dist", "built frontend directory")
	dev := flags.String("dev-origin", "", "optional loopback Vite origin")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected serve arguments")
	}
	if err := loopbackAddress(*addr); err != nil {
		return err
	}
	if *dev != "" {
		u, err := url.Parse(*dev)
		if err != nil || u.Scheme != "http" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("invalid development origin")
		}
		if err := loopbackAddress(u.Host); err != nil {
			return err
		}
	}
	store, err := storage.Open(*dir)
	if err != nil {
		return err
	}
	defer store.Close()
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	address := listener.Addr().String()
	_, port, _ := net.SplitHostPort(address)
	config := httpapi.Config{AllowedHosts: []string{address, net.JoinHostPort("localhost", port)}, AllowedOrigins: []string{"http://" + address, "http://localhost:" + port}}
	if *dev != "" {
		config.AllowedOrigins = append(config.AllowedOrigins, *dev)
	}
	root, err := os.OpenRoot(*web)
	if err == nil {
		defer root.Close()
		config.WebFS = root.FS()
	} else if !os.IsNotExist(err) {
		return err
	}
	bots := library.New(store)
	manager, err := jobs.New(store, bots)
	if err != nil {
		return err
	}
	defer manager.Close()
	config.Jobs = manager
	service := app.New(store, bots, app.Options{})
	server := &http.Server{Handler: httpapi.New(service, bots, config), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	if _, err := fmt.Fprintf(out, "EvoNardy listening at http://%s\n", address); err != nil {
		return err
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		// Closing active SSE streams prevents a long shutdown from retaining the data lock.
		_ = server.Close()
		err := <-result
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
func botsCommand(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 || (args[0] != "list" && args[0] != "save") {
		return fmt.Errorf("use bots list or bots save")
	}
	flags := flag.NewFlagSet("bots "+args[0], flag.ContinueOnError)
	flags.SetOutput(out)
	dir := flags.String("data-dir", "./data", "exclusive artifact directory")
	source := flags.String("source", library.HeuristicID, "source bot ID")
	name := flags.String("name", "Heuristic snapshot", "saved display name")
	if err := flags.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected bot arguments")
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
	var value any
	if args[0] == "list" {
		value, err = bots.List()
	} else {
		var b [16]byte
		_, err = rand.Read(b[:])
		if err == nil {
			value, err = bots.SaveCopy(library.SaveRequest{Command: library.Command{CommandID: hex.EncodeToString(b[:])}, SourceID: *source, Name: *name})
		}
	}
	if err != nil {
		return err
	}
	e := json.NewEncoder(out)
	e.SetIndent("", "  ")
	return e.Encode(value)
}
