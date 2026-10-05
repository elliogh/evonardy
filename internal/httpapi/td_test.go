package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"evonardy/internal/app"
	"evonardy/internal/game"
	"evonardy/internal/httpapi"
	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/neural"
	"evonardy/internal/replay"
	"evonardy/internal/storage"
	"evonardy/internal/training"
)

func TestNeuralHTTPTrainWatchPublishEvaluateAndPlay(t *testing.T) {
	for _, lambda := range []float64{0, 0.7} {
		cfg := training.DefaultTDConfig()
		cfg.Games, cfg.MaxTurns, cfg.Lambda = 2, 8, lambda
		t.Run(cfg.Algorithm(), func(t *testing.T) {
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
				data, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				r := httptest.NewRequest(method, path, bytes.NewReader(data))
				r.Host = "localhost"
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != 200 {
					t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body.String())
				}
				return w
			}
			req := jobs.StartRequest{Command: library.Command{CommandID: "http-neural"}, Name: "HTTP neural training", Algorithm: cfg.Algorithm(), TDConfig: &cfg}
			call("POST", "/api/training/runs", req)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			x, err := manager.Wait(ctx, req.CommandID)
			if err != nil || x.State != jobs.Completed || x.Counters.Updates != 16 || x.NeuralCandidate == nil {
				t.Fatal(x, err)
			}
			call("POST", "/api/training/runs", req)
			w := call("GET", "/api/training/runs/"+x.ID, nil)
			for _, private := range []string{"parameters", "td_training", "commands", "generation_hashes"} {
				if strings.Contains(w.Body.String(), `"`+private+`"`) {
					t.Fatal("private checkpoint data leaked", private)
				}
			}
			w = call("GET", "/api/training/runs/"+x.ID+"/watch", nil)
			var watched jobs.WatchedGame
			if err := json.Unmarshal(w.Body.Bytes(), &watched); err != nil {
				t.Fatal(err)
			}
			final, err := replay.Play(watched.Replay)
			if err != nil || len(watched.Positions) != 9 || watched.Positions[8] != final || watched.Replay.Bots != [2]string{cfg.Algorithm() + "/self-play", cfg.Algorithm() + "/self-play"} {
				t.Fatal("watch did not return actual TD self-play", err)
			}
			w = call("POST", "/api/training/runs/"+x.ID+"/save-bot", jobs.SaveRequest{Command: library.Command{CommandID: "http-neural-save", ExpectedVersion: x.Version}, Generation: x.NeuralCandidate.Game, CandidateID: x.NeuralCandidate.ID, Name: "HTTP neural model"})
			var card library.Card
			if err := json.Unmarshal(w.Body.Bytes(), &card); err != nil || card.Kind != neural.Version {
				t.Fatal("not a frozen neural model", err)
			}
			frozen, err := bots.Freeze(card.ID)
			if err != nil {
				t.Fatal(err)
			}
			ec := jobs.DefaultEvaluationConfig()
			ec.Pairs = 1
			call("POST", "/api/evaluations", jobs.EvaluateRequest{Command: library.Command{CommandID: "http-neural-eval"}, BotID: card.ID, Config: ec})
			y, err := manager.Wait(ctx, "http-neural-eval")
			if err != nil || y.State != jobs.Completed || y.Evaluation.Stats == nil || y.Evaluation.Stats.Games != 4 || y.Counters.Updates != 0 {
				t.Fatal(y, err)
			}
			// Ordinary game creation remains available after the trainer shuts down.
			if err := manager.Close(); err != nil {
				t.Fatal(err)
			}
			w = call("POST", "/api/games", app.CreateRequest{Command: app.Command{CommandID: "http-neural-play"}, BotID: card.ID, Human: game.White})
			var session app.Snapshot
			if err := json.Unmarshal(w.Body.Bytes(), &session); err != nil || session.BotID != card.ID || game.ValidatePosition(session.Position) != nil {
				t.Fatal("saved model could not play independently", err)
			}
			if after, err := bots.Freeze(card.ID); err != nil || after != frozen {
				t.Fatal("evaluation or play changed the published model")
			}
		})
	}
}
