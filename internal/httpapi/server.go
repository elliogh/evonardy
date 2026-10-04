// Package httpapi exposes a loopback-only, versioned application protocol.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"evonardy/internal/app"
	"evonardy/internal/jobs"
	"evonardy/internal/library"
)

type Config struct {
	AllowedHosts   []string
	AllowedOrigins []string
	WebFS          fs.FS
	Jobs           *jobs.Manager
}
type update struct {
	GameID  string `json:"game_id"`
	Version uint64 `json:"version"`
}
type hub struct {
	mu       sync.Mutex
	channels map[string]map[chan update]bool
}

func (h *hub) subscribe(id string) (chan update, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan update, 1)
	if h.channels[id] == nil {
		h.channels[id] = map[chan update]bool{}
	}
	h.channels[id][ch] = true
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		delete(h.channels[id], ch)
		if len(h.channels[id]) == 0 {
			delete(h.channels, id)
		}
	}
}
func (h *hub) publish(x app.Snapshot) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.channels[x.ID] {
		select {
		case ch <- update{x.ID, x.Version}:
		default:
			select {
			case <-ch:
			default:
			}
			ch <- update{x.ID, x.Version}
		}
	}
}

type server struct {
	games    *app.Service
	bots     *library.Library
	config   Config
	hub      hub
	requests chan struct{}
	streams  chan struct{}
}

func New(games *app.Service, bots *library.Library, config Config) http.Handler {
	s := &server{games: games, bots: bots, config: config, hub: hub{channels: map[string]map[chan update]bool{}}, requests: make(chan struct{}, 32), streams: make(chan struct{}, 64)}
	mux := http.NewServeMux()
	if config.Jobs != nil {
		s.jobRoutes(mux)
	}
	mux.HandleFunc("GET /api/bots", func(w http.ResponseWriter, r *http.Request) { cards, err := bots.List(); respond(w, cards, err) })
	mux.HandleFunc("GET /api/bots/{id...}", func(w http.ResponseWriter, r *http.Request) {
		card, err := bots.Get(r.PathValue("id"))
		respond(w, card, err)
	})
	mux.HandleFunc("POST /api/bots", func(w http.ResponseWriter, r *http.Request) {
		var req library.SaveRequest
		if !decode(w, r, &req) {
			return
		}
		card, err := bots.SaveCopy(req)
		respond(w, card, err)
	})
	mux.HandleFunc("PATCH /api/bots/{id}/metadata", func(w http.ResponseWriter, r *http.Request) {
		var req library.RenameRequest
		if !decode(w, r, &req) {
			return
		}
		card, err := bots.Rename(r.PathValue("id"), req)
		respond(w, card, err)
	})
	mux.HandleFunc("GET /api/games", func(w http.ResponseWriter, r *http.Request) {
		list, err := games.List(r.Context())
		respond(w, list, err)
	})
	mux.HandleFunc("POST /api/games", func(w http.ResponseWriter, r *http.Request) {
		var req app.CreateRequest
		if !decode(w, r, &req) {
			return
		}
		x, err := games.Create(r.Context(), req)
		s.changed(w, x, err)
	})
	mux.HandleFunc("GET /api/games/{id}", func(w http.ResponseWriter, r *http.Request) {
		x, err := games.Get(r.Context(), r.PathValue("id"))
		respond(w, x, err)
	})
	mux.HandleFunc("POST /api/games/{id}/roll", func(w http.ResponseWriter, r *http.Request) {
		var req app.Command
		if !decode(w, r, &req) {
			return
		}
		x, err := games.Roll(r.Context(), r.PathValue("id"), req)
		s.changed(w, x, err)
	})
	mux.HandleFunc("POST /api/games/{id}/continuations", func(w http.ResponseWriter, r *http.Request) {
		var req app.DraftRequest
		if !decode(w, r, &req) {
			return
		}
		x, err := games.SetDraft(r.Context(), r.PathValue("id"), req)
		s.changed(w, x, err)
	})
	mux.HandleFunc("POST /api/games/{id}/turn", func(w http.ResponseWriter, r *http.Request) {
		var req app.TurnRequest
		if !decode(w, r, &req) {
			return
		}
		x, err := games.Confirm(r.Context(), r.PathValue("id"), req)
		s.changed(w, x, err)
	})
	mux.HandleFunc("GET /api/games/{id}/events", s.events)
	mux.HandleFunc("GET /api/replays/{id}", func(w http.ResponseWriter, r *http.Request) {
		record, err := games.Replay(r.Context(), r.PathValue("id"))
		if err == nil {
			w.Header().Set("Content-Disposition", `attachment; filename="game-`+r.PathValue("id")+`.json"`)
		}
		respond(w, record, err)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		fail(w, http.StatusNotFound, "not_found", "Unknown API route")
	})
	if config.WebFS != nil {
		mux.Handle("/", http.FileServerFS(config.WebFS))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			fail(w, 503, "assets_unavailable", "Build the frontend with make build, or use make dev.")
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if !contains(config.AllowedHosts, r.Host) {
			fail(w, 403, "forbidden", "Host is not allowed")
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" && !contains(config.AllowedOrigins, origin) {
			fail(w, 403, "forbidden", "Origin is not allowed")
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			fail(w, 403, "forbidden", "Cross-site requests are not allowed")
			return
		}
		if strings.HasSuffix(r.URL.Path, "/events") && r.Method == "GET" {
			mux.ServeHTTP(w, r)
			return
		}
		select {
		case s.requests <- struct{}{}:
			defer func() { <-s.requests }()
		default:
			fail(w, 503, "busy", "Too many requests")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	media, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || media != "application/json" {
		fail(w, 415, "invalid_content_type", "Use application/json")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 65536)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		var limit *http.MaxBytesError
		if errors.As(err, &limit) {
			fail(w, 413, "too_large", "Request exceeds 64 KiB")
		} else {
			fail(w, 400, "invalid_json", "Cannot read request")
		}
		return false
	}
	if err := library.DecodeJSON(data, value); err != nil {
		fail(w, 400, "invalid_json", err.Error())
		return false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields["command_id"] == nil || fields["expected_version"] == nil || string(fields["expected_version"]) == "null" {
		fail(w, 400, "invalid_command", "Mutations require command_id and expected_version")
		return false
	}
	return true
}
func fail(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func respond(w http.ResponseWriter, value any, err error) {
	if err != nil {
		switch {
		case errors.Is(err, app.ErrNotFound), errors.Is(err, library.ErrNotFound), errors.Is(err, jobs.ErrNotFound):
			fail(w, 404, "not_found", err.Error())
		case errors.Is(err, app.ErrConflict), errors.Is(err, library.ErrConflict):
			fail(w, 409, "conflict", err.Error())
		case errors.Is(err, app.ErrInvalid), errors.Is(err, library.ErrInvalid):
			fail(w, 400, "invalid_command", err.Error())
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			fail(w, 503, "cancelled", "Request cancelled; retry the same command_id")
		default:
			log.Printf("application error: %v", err)
			fail(w, 500, "internal_error", "Storage or application error; retry the same command_id")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}
func (s *server) changed(w http.ResponseWriter, x app.Snapshot, err error) {
	if err == nil {
		s.hub.publish(x)
	}
	respond(w, x, err)
}
func (s *server) events(w http.ResponseWriter, r *http.Request) {
	select {
	case s.streams <- struct{}{}:
		defer func() { <-s.streams }()
	default:
		fail(w, 503, "busy", "Too many event streams")
		return
	}
	id := r.PathValue("id")
	ch, unsubscribe := s.hub.subscribe(id)
	defer unsubscribe()
	x, err := s.games.Get(r.Context(), id)
	if err != nil {
		respond(w, nil, err)
		return
	}
	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func(value *update) error {
		if err := controller.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		if value == nil {
			_, err = fmt.Fprint(w, ": keepalive\n\n")
		} else {
			data, _ := json.Marshal(value)
			_, err = fmt.Fprintf(w, "event: game.updated\ndata: %s\n\n", data)
		}
		if err != nil {
			return err
		}
		return controller.Flush()
	}
	if err := send(&update{x.ID, x.Version}); err != nil {
		return
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case value := <-ch:
			if send(&value) != nil {
				return
			}
		case <-ticker.C:
			if send(nil) != nil {
				return
			}
		}
	}
}
