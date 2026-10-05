package httpapi

import (
	"evonardy/internal/library"
	"evonardy/internal/research"
	"net/http"
)

func (s *server) researchRoutes(mux *http.ServeMux) {
	m := s.config.Research
	mux.HandleFunc("GET /api/experiments/config", func(w http.ResponseWriter, r *http.Request) { respond(w, research.DefaultConfig(), nil) })
	mux.HandleFunc("GET /api/experiments", func(w http.ResponseWriter, r *http.Request) { x, err := m.List(); respond(w, x, err) })
	mux.HandleFunc("POST /api/experiments", func(w http.ResponseWriter, r *http.Request) {
		var req research.StartRequest
		if !decode(w, r, &req) {
			return
		}
		x, err := m.Start(r.Context(), req)
		respond(w, x, err)
	})
	mux.HandleFunc("GET /api/experiments/{id}", func(w http.ResponseWriter, r *http.Request) { x, err := m.Get(r.PathValue("id")); respond(w, x, err) })
	for _, op := range []string{"stop", "resume"} {
		mux.HandleFunc("POST /api/experiments/{id}/"+op, func(w http.ResponseWriter, r *http.Request) {
			var cmd library.Command
			if !decode(w, r, &cmd) {
				return
			}
			var x research.Snapshot
			var err error
			if op == "stop" {
				x, err = m.Stop(r.Context(), r.PathValue("id"), cmd)
			} else {
				x, err = m.Resume(r.Context(), r.PathValue("id"), cmd)
			}
			respond(w, x, err)
		})
	}
	mux.HandleFunc("GET /api/experiments/{id}/report", func(w http.ResponseWriter, r *http.Request) {
		data, err := m.Report(r.PathValue("id"))
		if err == nil {
			w.Header().Set("Content-Disposition", "attachment; filename=research-report.json")
		}
		respond(w, data, err)
	})
}
