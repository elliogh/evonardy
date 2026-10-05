package jobs

import (
	"context"
	"testing"
	"time"

	"evonardy/internal/library"
	"evonardy/internal/storage"
	"evonardy/internal/training"
)

func TestQueuedNeuralCheckpointRecoversBeforeFirstGame(t *testing.T) {
	store, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	c := training.DefaultTDConfig()
	c.Games, c.MaxTurns = 1, 1
	state, err := training.NewTD(c)
	if err != nil {
		t.Fatal(err)
	}
	req := StartRequest{Command: library.Command{CommandID: "queued-neural"}, Name: "Queued neural", TDConfig: &c}
	r := initial(req.CommandID, Training, req.Name, fingerprint("start", req))
	r.TDTraining = &state
	// Write the same initial envelope as Start, without launching a worker: this
	// represents a crash after queue publication and before the first game.
	writer := &Manager{store: store, records: map[string]record{}, changed: make(chan struct{})}
	if err := writer.persist(r); err != nil {
		t.Fatal(err)
	}
	m, err := New(store, library.New(store))
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	x, err := m.Get(r.ID)
	if err != nil || x.State != Interrupted || x.Generation != 0 || x.Counters.Games != 0 || !x.CanResume {
		t.Fatal("initial neural boundary did not recover")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := m.Resume(ctx, x.ID, library.Command{CommandID: "resume-queued", ExpectedVersion: x.Version}); err != nil {
		t.Fatal(err)
	}
	if done, err := m.Wait(ctx, x.ID); err != nil || done.State != Completed || done.Counters.Games != 1 {
		t.Fatal("initial boundary could not resume")
	}
}
