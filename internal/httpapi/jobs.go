package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"evonardy/internal/jobs"
	"evonardy/internal/library"
)

func (s *server) jobRoutes(mux *http.ServeMux) {
	m := s.config.Jobs
	mux.HandleFunc("GET /api/training/runs/{id}/watch", func(w http.ResponseWriter, r *http.Request) { x, err := m.Watch(r.PathValue("id")); respond(w, x, err) })
	mux.HandleFunc("POST /api/training/runs", func(w http.ResponseWriter, r *http.Request) {
		var req jobs.StartRequest
		if !decode(w, r, &req) {
			return
		}
		x, err := m.Start(r.Context(), req)
		respond(w, x, err)
	})
	mux.HandleFunc("POST /api/evaluations", func(w http.ResponseWriter, r *http.Request) {
		var req jobs.EvaluateRequest
		if !decode(w, r, &req) {
			return
		}
		x, err := m.Evaluate(r.Context(), req)
		respond(w, x, err)
	})
	for _, route := range []struct{ path, kind string }{{"/api/training/runs", jobs.Training}, {"/api/evaluations", jobs.Evaluation}} {
		mux.HandleFunc("GET "+route.path, func(w http.ResponseWriter, r *http.Request) { x, err := m.List(route.kind); respond(w, x, err) })
		mux.HandleFunc("GET "+route.path+"/{id}", func(w http.ResponseWriter, r *http.Request) {
			x, err := m.Get(r.PathValue("id"))
			if err == nil && x.Kind != route.kind {
				err = jobs.ErrNotFound
			}
			respond(w, x, err)
		})
		for _, op := range []string{"stop", "resume"} {
			mux.HandleFunc("POST "+route.path+"/{id}/"+op, func(w http.ResponseWriter, r *http.Request) {
				var cmd library.Command
				if !decode(w, r, &cmd) {
					return
				}
				x, err := m.Get(r.PathValue("id"))
				if err != nil || x.Kind != route.kind {
					respond(w, nil, jobs.ErrNotFound)
					return
				}
				if op == "stop" {
					x, err = m.Stop(r.Context(), x.ID, cmd)
				} else {
					x, err = m.Resume(r.Context(), x.ID, cmd)
				}
				respond(w, x, err)
			})
		}
		mux.HandleFunc("GET "+route.path+"/{id}/events", func(w http.ResponseWriter, r *http.Request) { s.jobEvents(w, r, route.kind) })
	}
	mux.HandleFunc("POST /api/training/runs/{id}/save-bot", func(w http.ResponseWriter, r *http.Request) {
		var req jobs.SaveRequest
		if !decode(w, r, &req) {
			return
		}
		x, err := m.Save(r.Context(), r.PathValue("id"), req)
		respond(w, x, err)
	})
	mux.HandleFunc("GET /api/training/runs/{id}/generations/{generation}", func(w http.ResponseWriter, r *http.Request) {
		n, err := strconv.Atoi(r.PathValue("generation"))
		if err != nil {
			respond(w, nil, jobs.ErrInvalid)
			return
		}
		x, err := m.Generation(r.PathValue("id"), n)
		respond(w, x, err)
	})
}
func (s *server) jobEvents(w http.ResponseWriter, r *http.Request, kind string) {
	select {
	case s.streams <- struct{}{}:
		defer func() { <-s.streams }()
	default:
		fail(w, 503, "busy", "Too many event streams")
		return
	}
	id := r.PathValue("id")
	x, err := s.config.Jobs.Status(id)
	if err != nil || x.Kind != kind {
		respond(w, nil, jobs.ErrNotFound)
		return
	}
	controller := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	send := func(event string, value any) error {
		if err := controller.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		var err error
		if value == nil {
			_, err = fmt.Fprint(w, ": keepalive\n\n")
		} else {
			data, _ := json.Marshal(value)
			_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		}
		if err != nil {
			return err
		}
		return controller.Flush()
	}
	event := func(state string) string {
		switch state {
		case jobs.Completed:
			return "run.completed"
		case jobs.Failed:
			return "run.failed"
		case jobs.Stopped, jobs.Interrupted:
			return "checkpoint.saved"
		default:
			return "run.progress"
		}
	}
	if send(event(x.State), x) != nil {
		return
	}
	revision := x.Revision
	progress := time.NewTicker(500 * time.Millisecond)
	defer progress.Stop()
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-progress.C:
			y, err := s.config.Jobs.Status(id)
			if err != nil {
				return
			}
			if y.Revision != revision {
				if send(event(y.State), y) != nil {
					return
				}
				revision = y.Revision
			}
		case <-keepalive.C:
			if send("", nil) != nil {
				return
			}
		}
	}
}
