package library_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"evonardy/internal/agent"
	"evonardy/internal/encoder"
	"evonardy/internal/game"
	"evonardy/internal/library"
	"evonardy/internal/neural"
	"evonardy/internal/storage"
)

func TestPublishedNeuralInferenceSurvivesEditsRenameAndRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	l := library.New(store)
	parameters := neural.Initialize(42, 7).Parameters()
	original, err := agent.NewNeural(parameters)
	if err != nil {
		t.Fatal(err)
	}
	card, err := l.SaveNeural("Frozen neural model", parameters, "test learner", "run/snapshot")
	if err != nil || card.Kind != neural.Version {
		t.Fatalf("publish: %+v, %v", card, err)
	}
	detail, err := l.Get(card.ID)
	if err != nil || detail.Manifest.FeaturesVersion != encoder.Version || detail.Manifest.Evaluator != neural.Version {
		t.Fatal("missing versioned neural contract")
	}
	manifestPath, modelPath := filepath.Join(dir, "bots", card.ID, "manifest.json"), filepath.Join(dir, "bots", card.ID, "model.json")
	manifestBefore, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	modelBefore, err := os.ReadFile(modelPath)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := l.Freeze(card.ID)
	if err != nil {
		t.Fatal(err)
	}
	frozen := policy.New(nil)
	if frozen == nil {
		t.Fatal("frozen neural model required a learner or RNG")
	}
	// Simulate further learner updates and edits to a returned policy copy.
	parameters[0] += .5
	policy.NeuralParameters[0] += 4
	updated, err := l.SaveNeural("Later snapshot", parameters, "test learner", card.ID)
	if err != nil || updated.ID == card.ID {
		t.Fatal("new parameters must have a different identity")
	}
	renamed, err := l.Rename(card.ID, library.RenameRequest{Command: library.Command{CommandID: "rename-neural", ExpectedVersion: card.MetadataVersion}, Name: "Renamed neural model"})
	if err != nil || renamed.ID != card.ID {
		t.Fatalf("rename changed inference identity: %v", err)
	}
	copyCard, err := l.SaveCopy(library.SaveRequest{Command: library.Command{CommandID: "copy-neural"}, SourceID: card.ID, Name: "Same strategy"})
	if err != nil || copyCard.ID != card.ID {
		t.Fatalf("identical neural strategies must deduplicate: %v", err)
	}
	for path, want := range map[string][]byte{manifestPath: manifestBefore, modelPath: modelBefore} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("learner/rename altered a published inference file")
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	l = library.New(store)
	reloaded, err := l.Agent(card.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	for side := game.White; side <= game.Black; side++ {
		for roll := 1; roll <= 6; roll++ {
			p := game.Initial(side)
			p.FirstDone = [2]bool{true, true}
			p.Checkers[side][game.Head(side)] = 13
			p.Checkers[side][game.PhysicalPoint(side, 2)] = 1
			p.Checkers[side][game.PhysicalPoint(side, 5)] = 1
			dice := game.Dice{roll, (roll+2)%6 + 1}
			actions, err := game.LegalActions(p, dice)
			if err != nil {
				t.Fatal(err)
			}
			want, err := original.Choose(context.Background(), p, dice, actions)
			if err != nil {
				t.Fatal(err)
			}
			for _, bot := range []agent.Agent{frozen, reloaded} {
				got, err := bot.Choose(context.Background(), p, dice, actions)
				if err != nil || got != want {
					t.Fatalf("side %d, roll %d: snapshot/restart changed decision: %d, %d, %v", side, roll, want, got, err)
				}
			}
		}
	}
}

func TestNeuralCorruptionAndOversizedFilesAreRejected(t *testing.T) {
	for _, test := range []struct {
		name string
		file string
		data []byte
	}{
		{"checksum corruption", "model.json", []byte(`{"weights":[1]}`)},
		{"oversized model", "model.json", bytes.Repeat([]byte(" "), 65537)},
		{"oversized manifest", "manifest.json", bytes.Repeat([]byte(" "), 65537)},
		{"malformed manifest", "manifest.json", []byte(`{"unknown":true}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			store, err := storage.Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			l := library.New(store)
			card, err := l.SaveNeural("Corruption fixture", neural.Initialize(42, 0).Parameters(), "test", "")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "bots", card.ID, test.file), test.data, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := l.Agent(card.ID, nil); err == nil {
				t.Fatal("loaded corrupt/oversized package")
			}
			if _, err := l.Freeze(card.ID); err == nil {
				t.Fatal("froze corrupt/oversized package")
			}
			cards, err := l.List()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, c := range cards {
				if c.ID == card.ID {
					found = !c.Available && c.Reason != ""
				}
			}
			if !found {
				t.Fatal("library must show why the model cannot play")
			}
		})
	}
}
