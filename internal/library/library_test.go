package library_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"evonardy/internal/agent"
	"evonardy/internal/features"
	"evonardy/internal/game"
	"evonardy/internal/library"
	"evonardy/internal/random"
	"evonardy/internal/storage"
)

func TestSaveLoadRenamePreservesInferenceAndIdentity(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	l := library.New(s)
	card, err := l.SaveCopy(library.SaveRequest{Command: library.Command{CommandID: "save-one", ExpectedVersion: 0}, SourceID: library.HeuristicID, Name: "My baseline"})
	if err != nil {
		t.Fatal(err)
	}
	if card.Builtin || !card.Available || card.Kind != "linear" {
		t.Fatalf("card: %+v", card)
	}
	again, err := l.SaveCopy(library.SaveRequest{Command: library.Command{CommandID: "save-one"}, SourceID: library.HeuristicID, Name: "My baseline"})
	if err != nil || again.ID != card.ID {
		t.Fatal("save is not idempotent")
	}
	renamed, err := l.Rename(card.ID, library.RenameRequest{Command: library.Command{CommandID: "rename-one", ExpectedVersion: card.MetadataVersion}, Name: "Renamed baseline"})
	if err != nil || renamed.ID != card.ID || renamed.Name != "Renamed baseline" {
		t.Fatalf("rename: %+v %v", renamed, err)
	}
	if _, err := l.Rename(card.ID, library.RenameRequest{Command: library.Command{CommandID: "rename-stale", ExpectedVersion: card.MetadataVersion}, Name: "Stale"}); err == nil {
		t.Fatal("stale metadata accepted")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	l = library.New(s)
	loaded, err := l.Get(card.ID)
	if err != nil || loaded.Name != renamed.Name {
		t.Fatal("library lost after restart")
	}
	bot, err := l.Agent(card.ID, random.New(9, "bot", 0))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		p := game.Initial(game.Player(i % 2))
		p.FirstDone = [2]bool{true, true}
		p.Checkers[p.Turn][game.Head(p.Turn)] = 13
		p.Checkers[p.Turn][game.PhysicalPoint(p.Turn, 2)] = 1
		p.Checkers[p.Turn][game.PhysicalPoint(p.Turn, 5)] = 1
		dice := game.Dice{i%6 + 1, (i+2)%6 + 1}
		actions, err := game.LegalActions(p, dice)
		if err != nil {
			t.Fatal(err)
		}
		a, err := (agent.Heuristic{}).Choose(context.Background(), p, dice, actions)
		if err != nil {
			t.Fatal(err)
		}
		b, err := bot.Choose(context.Background(), p, dice, actions)
		if err != nil || a != b {
			t.Fatal("saving changed inference")
		}
	}
	weights := append([]float64{}, features.DefaultWeights[:]...)
	weights[0] += .25
	other, err := l.SaveLinear("Other model", weights, "manual test", "test-parent")
	if err != nil || other.ID == card.ID {
		t.Fatal("new weights did not get a new identity")
	}
	if _, err := l.Get(card.ID); err != nil {
		t.Fatal("new model damaged old model")
	}
}

func TestRandomSnapshotPreservesIndependentAgentChoices(t *testing.T) {
	s, err := storage.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	l := library.New(s)
	card, err := l.SaveCopy(library.SaveRequest{Command: library.Command{CommandID: "random-copy"}, SourceID: library.RandomID, Name: "Random snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	for side := game.White; side <= game.Black; side++ {
		p := game.Initial(side)
		actions, err := game.LegalActions(p, game.Dice{3, 5})
		if err != nil {
			t.Fatal(err)
		}
		for index := uint64(0); index < 20; index++ {
			before, err := l.Agent(library.RandomID, random.New(42, "agent", index))
			if err != nil {
				t.Fatal(err)
			}
			after, err := l.Agent(card.ID, random.New(42, "agent", index))
			if err != nil {
				t.Fatal(err)
			}
			a, err := before.Choose(context.Background(), p, game.Dice{3, 5}, actions)
			if err != nil {
				t.Fatal(err)
			}
			b, err := after.Choose(context.Background(), p, game.Dice{3, 5}, actions)
			if err != nil || a != b {
				t.Fatal("saved random strategy changed selection")
			}
		}
	}
}

func TestMalformedAndIncompatibleModelsAreRejectedAndListed(t *testing.T) {
	dir := t.TempDir()
	s, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	l := library.New(s)
	for _, weights := range [][]float64{{1}, {math.NaN()}, {math.Inf(1)}, {1, 2, 3, 4, 5, 6, 7, 8, math.Inf(1)}} {
		if _, err := l.SaveLinear("Bad", weights, "test", ""); err == nil {
			t.Fatal("invalid weights accepted")
		}
	}
	card, err := l.SaveCopy(library.SaveRequest{Command: library.Command{CommandID: "saved"}, SourceID: library.HeuristicID, Name: "Snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bots", card.ID, "model.json"), []byte(`{"weights":[1]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := l.Agent(card.ID, random.New(1, "bot", 0)); err == nil {
		t.Fatal("corrupt model loaded")
	}
	cards, err := l.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(cards) != 3 {
		t.Fatalf("missing incompatible card: %+v", cards)
	}
	for _, c := range cards {
		if c.ID == card.ID && (c.Available || c.Reason == "") {
			t.Fatal("incompatibility hidden")
		}
	}
}
