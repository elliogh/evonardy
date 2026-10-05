package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"evonardy/internal/app"
	"evonardy/internal/httpapi"
	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/research"
	"evonardy/internal/storage"
	"evonardy/internal/training"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResearchHTTPProtocol(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	bots := library.New(store)
	m, err := research.New(store, bots)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	h := httpapi.New(app.New(store, bots, app.Options{}), bots, httpapi.Config{AllowedHosts: []string{"localhost"}, Research: m})
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Host = "localhost"
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := call("GET", "/api/experiments/config", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"training_seeds":[42,43,44,45,46]`) {
		t.Fatal("missing five-seed default", w.Body.String())
	}
	cfg := research.DefaultConfig()
	td := training.DefaultTDConfig()
	td.Games = 1
	cfg.Methods = []research.Method{{Algorithm: "td0", TD: &td}}
	cfg.TrainingSeeds = []uint64{42}
	cfg.DevelopmentPairs = 1
	cfg.Confirmation.Pairs = 1
	cfg.Bootstrap.Resamples = 100
	cfg.Confirmation.Bootstrap.Resamples = 100
	req := research.StartRequest{Command: library.Command{CommandID: "http-research"}, Name: "HTTP research", Config: cfg}
	w = call("POST", "/api/experiments", req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	x, err := m.Wait(ctx, req.CommandID)
	if err != nil || x.State != jobs.Completed {
		t.Fatal(x.Error, err)
	}
	for _, route := range []string{"/api/experiments", "/api/experiments/" + x.ID, "/api/experiments/" + x.ID + "/report"} {
		w = call("GET", route, nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), x.ID) {
			t.Fatal(route, w.Code, w.Body.String())
		}
	}
	if w = call("POST", "/api/experiments/"+x.ID+"/resume", library.Command{CommandID: "final-again", ExpectedVersion: x.Version}); w.Code != 409 {
		t.Fatal("final confirmation repeated")
	}
	if w = call("GET", "/api/experiments/missing", nil); w.Code != 404 {
		t.Fatal("missing experiment status")
	}
	req.CommandID = "reuse"
	w = call("POST", "/api/experiments", req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"final_data_role":"reused-development"`) {
		t.Fatal("undisclosed reused final schedule", w.Body.String())
	}
	req.CommandID = "bad-config"
	req.Config.FinalSeed = req.Config.DevelopmentSeed
	if w = call("POST", "/api/experiments", req); w.Code != 400 {
		t.Fatal("accepted overlapping phase roots")
	}
	if w = call("POST", "/api/experiments", map[string]any{"unexpected": true}); w.Code != 400 {
		t.Fatal("accepted unknown request field")
	}
}
