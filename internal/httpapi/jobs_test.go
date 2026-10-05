package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"evonardy/internal/app"
	"evonardy/internal/httpapi"
	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/storage"
	"evonardy/internal/training"
)

func TestTrainingHTTPPublicationAndEvaluation(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	bots := library.New(store)
	manager, err := jobs.New(store, bots)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	handler := httpapi.New(app.New(store, bots, app.Options{}), bots, httpapi.Config{AllowedHosts: []string{"localhost"}, Jobs: manager})
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Host = "localhost"
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	cfg := training.DefaultConfig()
	cfg.Population = 4
	cfg.Generations = 1
	cfg.PairsPerOpponent = 1
	req := jobs.StartRequest{Command: library.Command{CommandID: "http-train"}, Name: "HTTP training", Config: cfg}
	w := call("POST", "/api/training/runs", req)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	x, err := manager.Wait(ctx, "http-train")
	if err != nil || x.State != jobs.Completed {
		t.Fatal(x, err)
	}
	w = call("POST", "/api/training/runs", req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"id":"http-train"`) {
		t.Fatal(w.Body.String())
	}
	if w = call("POST", "/api/training/runs/http-train/stop", library.Command{CommandID: "stale"}); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call("GET", "/api/training/runs/http-train/generations/1", nil); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w = call("GET", "/api/training/runs/http-train/watch", nil)
	if w.Code != 200 {
		t.Fatal("missing training game", w.Code, w.Body.String())
	}
	var watched jobs.WatchedGame
	if err := json.Unmarshal(w.Body.Bytes(), &watched); err != nil {
		t.Fatal(err)
	}
	if watched.Key != x.WatchedGame.Key || len(watched.Positions) != watched.Turns+1 || watched.Positions[0].Checkers[0][0] != 15 || watched.Replay.Outcome == nil {
		t.Fatal("invalid watch snapshot")
	}
	if w = call("GET", "/api/training/runs/missing/watch", nil); w.Code != 404 {
		t.Fatal("invented missing game")
	}
	save := jobs.SaveRequest{Command: library.Command{CommandID: "http-save", ExpectedVersion: x.Version}, Generation: 1, CandidateID: x.Candidates[1].ID, Name: "HTTP bot"}
	w = call("POST", "/api/training/runs/http-train/save-bot", save)
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var card library.Card
	json.Unmarshal(w.Body.Bytes(), &card)
	ec := jobs.DefaultEvaluationConfig()
	ec.Pairs = 1
	w = call("POST", "/api/evaluations", jobs.EvaluateRequest{Command: library.Command{CommandID: "http-eval"}, BotID: card.ID, Config: ec})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	y, err := manager.Wait(ctx, "http-eval")
	if err != nil || y.Evaluation.Stats == nil || y.Evaluation.Stats.Games != 4 {
		t.Fatal(y, err)
	}
	if w = call("GET", "/api/training/runs/http-eval", nil); w.Code != 404 {
		t.Fatal("cross-kind route", w.Code)
	}
	if w = call("GET", "/api/evaluations/http-eval", nil); w.Code != 200 || strings.Contains(w.Body.String(), "frozen_evaluation") {
		t.Fatal(w.Body.String())
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Host = "localhost"; handler.ServeHTTP(w, r) }))
	defer server.Close()
	streamCtx, stop := context.WithTimeout(ctx, 5*time.Second)
	defer stop()
	request, _ := http.NewRequestWithContext(streamCtx, "GET", server.URL+"/api/training/runs/http-train/events", nil)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	var event, data string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: ") {
			event = line
		}
		if strings.HasPrefix(line, "data: ") {
			data = line
			break
		}
	}
	if event != "event: run.completed" || !strings.Contains(data, `"run_id":"http-train"`) || !strings.Contains(data, `"revision":`) || strings.Contains(data, "candidates") {
		t.Fatal("invalid SSE notice", event, data)
	}
	stop()
	response.Body.Close()
}
