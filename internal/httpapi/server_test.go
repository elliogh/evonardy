package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"evonardy/internal/app"
	"evonardy/internal/game"
	"evonardy/internal/httpapi"
	"evonardy/internal/library"
	"evonardy/internal/storage"
)

func TestHTTPAuthorityVersionsSecurityAndSSE(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	bots := library.New(store)
	service := app.New(store, bots, app.Options{Seed: func() (uint64, error) { return 42, nil }})
	handler := httpapi.New(service, bots, httpapi.Config{AllowedHosts: []string{"127.0.0.1:8080"}, AllowedOrigins: []string{"http://127.0.0.1:8080"}})
	request := func(method, path, body, host, origin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Host = host
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := request("GET", "/api/bots", "", "attacker.example", ""); w.Code != 403 {
		t.Fatal("unsafe Host accepted")
	}
	if w := request("POST", "/api/games", "{}", "127.0.0.1:8080", "https://attacker.example"); w.Code != 403 {
		t.Fatal("unsafe Origin accepted")
	}
	if w := request("POST", "/api/games", strings.Repeat("x", 65537), "127.0.0.1:8080", ""); w.Code != 413 {
		t.Fatalf("body limit: %d", w.Code)
	}
	if w := request("POST", "/api/games", `{"command_id":"bad","expected_version":0,"human":0,"bot_id":"builtin/heuristic-v1","dice":[6,6]}`, "127.0.0.1:8080", ""); w.Code != 400 {
		t.Fatal("client assigned dice")
	}
	body := `{"command_id":"http-game","expected_version":0,"human":0,"bot_id":"builtin/heuristic-v1"}`
	if w := request("POST", "/api/games", `{"command_id":"missing-version","human":0,"bot_id":"builtin/heuristic-v1"}`, "127.0.0.1:8080", ""); w.Code != 400 {
		t.Fatal("missing expected_version accepted")
	}
	w := request("POST", "/api/games", body, "127.0.0.1:8080", "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var x app.Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &x); err != nil {
		t.Fatal(err)
	}
	original := append([]byte{}, w.Body.Bytes()...)
	w = request("POST", "/api/games", body, "127.0.0.1:8080", "")
	if !bytes.Equal(original, w.Body.Bytes()) {
		t.Fatal("repeated HTTP creation changed game")
	}
	if strings.Contains(w.Body.String(), `"seed"`) || strings.Contains(w.Body.String(), `"commands"`) {
		t.Fatal("private state exposed")
	}
	if x.Phase == app.AwaitingRoll {
		x, err = service.Roll(context.Background(), x.ID, app.Command{CommandID: "roll", ExpectedVersion: x.Version})
		if err != nil {
			t.Fatal(err)
		}
	}
	w = request("POST", "/api/games/"+x.ID+"/continuations", `{"command_id":"stale","expected_version":0,"prefix":[]}`, "127.0.0.1:8080", "")
	if w.Code != 409 {
		t.Fatal("stale command accepted")
	}

	// Stream against a real socket: an accepted mutation must arrive without polling.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { r.Host = "127.0.0.1:8080"; handler.ServeHTTP(w, r) }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/games/"+x.ID+"/events", nil)
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	scanner := bufio.NewScanner(response.Body)
	readEvent := func() string {
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "data: ") {
				return scanner.Text()
			}
		}
		t.Fatalf("missing SSE event: %v", scanner.Err())
		return ""
	}
	if !strings.Contains(readEvent(), fmt.Sprintf(`"version":%d`, x.Version)) {
		t.Fatal("SSE initial version missing")
	}
	data, _ := json.Marshal(app.DraftRequest{Command: app.Command{CommandID: "http-draft", ExpectedVersion: x.Version}, Prefix: []game.Step{x.Continuations.Next[0]}})
	w = request("POST", "/api/games/"+x.ID+"/continuations", string(data), "127.0.0.1:8080", "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	if !strings.Contains(readEvent(), fmt.Sprintf(`"version":%d`, x.Version+1)) {
		t.Fatal("SSE mutation missing")
	}
	cancel()
	response.Body.Close()
}

func TestBotMetadataHTTP(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	bots := library.New(store)
	handler := httpapi.New(app.New(store, bots, app.Options{}), bots, httpapi.Config{AllowedHosts: []string{"localhost"}})
	call := func(method, path string, body any) *httptest.ResponseRecorder {
		data, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, bytes.NewReader(data))
		r.Host = "localhost"
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	w := call("POST", "/api/bots", library.SaveRequest{Command: library.Command{CommandID: "save"}, SourceID: library.HeuristicID, Name: "Snapshot"})
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	var card library.Card
	json.Unmarshal(w.Body.Bytes(), &card)
	w = call("PATCH", "/api/bots/"+card.ID+"/metadata", library.RenameRequest{Command: library.Command{CommandID: "rename", ExpectedVersion: card.MetadataVersion}, Name: "Renamed"})
	if w.Code != 200 || !strings.Contains(w.Body.String(), "Renamed") {
		t.Fatal(w.Body.String())
	}
	w = call("PATCH", "/api/bots/"+card.ID+"/metadata", library.RenameRequest{Command: library.Command{CommandID: "stale", ExpectedVersion: card.MetadataVersion}, Name: "Lost"})
	if w.Code != 409 {
		t.Fatal("stale rename accepted")
	}
}
