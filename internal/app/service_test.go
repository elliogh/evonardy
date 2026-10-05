package app_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"evonardy/internal/app"
	"evonardy/internal/game"
	"evonardy/internal/library"
	"evonardy/internal/replay"
	"evonardy/internal/storage"
)

func service(t *testing.T, dir string) (*storage.Store, *app.Service) {
	t.Helper()
	s, err := storage.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s, app.New(s, library.New(s), app.Options{Seed: func() (uint64, error) { return 42, nil }})
}

func TestConcurrentCommandsAndCancellationPreserveOneVersion(t *testing.T) {
	store, s := service(t, t.TempDir())
	defer store.Close()
	ctx := context.Background()
	x, err := s.Create(ctx, app.CreateRequest{Command: app.Command{CommandID: "concurrent"}, Human: game.White, BotID: library.HeuristicID})
	if err != nil {
		t.Fatal(err)
	}
	if x.Phase == app.AwaitingRoll {
		x, err = s.Roll(ctx, x.ID, app.Command{CommandID: "roll", ExpectedVersion: x.Version})
		if err != nil {
			t.Fatal(err)
		}
	}
	request := app.DraftRequest{Command: app.Command{CommandID: "same", ExpectedVersion: x.Version}, Prefix: []game.Step{x.Continuations.Next[0]}}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.SetDraft(cancelled, x.ID, request); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled command accepted")
	}
	before, err := s.Get(ctx, x.ID)
	if err != nil || !reflect.DeepEqual(before, x) {
		t.Fatal("cancelled command persisted")
	}
	var wg sync.WaitGroup
	results := make(chan app.Snapshot, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			y, err := s.SetDraft(ctx, x.ID, request)
			if err != nil {
				failures <- err
			} else {
				results <- y
			}
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	for y := range results {
		if y.Version != x.Version+1 || len(y.Draft) != 1 {
			t.Fatal("concurrent retry executed twice")
		}
	}
	y, err := s.Get(ctx, x.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Returned data is a copy, including nested history and dice.
	*y.Dice = game.Dice{6, 6}
	y.Draft[0].From = 24
	after, err := s.Get(ctx, x.ID)
	if err != nil || after.Draft[0].From == 24 || *after.Dice != *x.Dice {
		t.Fatal("caller modified authoritative state")
	}
}

func TestPersistentDiceDraftAndIdempotentCommands(t *testing.T) {
	dir := t.TempDir()
	store, s := service(t, dir)
	ctx := context.Background()
	req := app.CreateRequest{Command: app.Command{CommandID: "create-one"}, BotID: library.HeuristicID, Human: game.White}
	x, err := s.Create(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if x.Phase == app.AwaitingRoll {
		x, err = s.Roll(ctx, x.ID, app.Command{CommandID: "roll-one", ExpectedVersion: x.Version})
		if err != nil {
			t.Fatal(err)
		}
	}
	if x.Dice == nil {
		t.Fatal("missing human dice")
	}
	if _, err := s.Roll(ctx, x.ID, app.Command{CommandID: "reroll", ExpectedVersion: x.Version}); err == nil {
		t.Fatal("existing dice rerolled")
	}
	oldVersion := x.Version
	draftReq := app.DraftRequest{Command: app.Command{CommandID: "draft-one", ExpectedVersion: x.Version}, Prefix: []game.Step{x.Continuations.Next[0]}}
	x, err = s.SetDraft(ctx, x.ID, draftReq)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.SetDraft(ctx, x.ID, draftReq)
	if err != nil || !reflect.DeepEqual(x, again) {
		t.Fatal("repeated draft changed state")
	}
	if _, err := s.SetDraft(ctx, x.ID, app.DraftRequest{Command: app.Command{CommandID: "stale", ExpectedVersion: oldVersion}, Prefix: nil}); !errors.Is(err, app.ErrConflict) {
		t.Fatal("stale command accepted")
	}
	draftReq.Prefix = nil
	if _, err := s.SetDraft(ctx, x.ID, draftReq); !errors.Is(err, app.ErrConflict) {
		t.Fatal("command id reused with a different body")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, s = service(t, dir)
	defer store.Close()
	restored, err := s.Get(ctx, x.ID)
	if err != nil || !reflect.DeepEqual(x, restored) {
		t.Fatalf("restart changed draft/dice: %v", err)
	}
	createdAgain, err := s.Create(ctx, req)
	if err != nil || createdAgain.ID != x.ID || !reflect.DeepEqual(createdAgain, restored) {
		t.Fatal("repeated create rerolled or reset a session")
	}
	undone, err := s.SetDraft(ctx, x.ID, app.DraftRequest{Command: app.Command{CommandID: "undo-one", ExpectedVersion: x.Version}, Prefix: []game.Step{}})
	if err != nil || len(undone.Draft) != 0 || undone.Position != undone.DraftPosition || *undone.Dice != *x.Dice {
		t.Fatal("undo changed authoritative board or dice")
	}
}

func TestHumanGameCompletesAndReplaySurvivesRestart(t *testing.T) {
	for _, human := range []game.Player{game.White, game.Black} {
		t.Run(fmt.Sprint(human), func(t *testing.T) {
			dir := t.TempDir()
			store, s := service(t, dir)
			ctx := context.Background()
			x, err := s.Create(ctx, app.CreateRequest{Command: app.Command{CommandID: "full-game"}, Human: human, BotID: library.HeuristicID})
			if err != nil {
				t.Fatal(err)
			}
			counter := 0
			command := func() app.Command {
				counter++
				return app.Command{CommandID: fmt.Sprintf("cmd-%d", counter), ExpectedVersion: x.Version}
			}
			for x.Phase != app.Finished && counter < 3000 {
				if x.Phase == app.AwaitingRoll {
					x, err = s.Roll(ctx, x.ID, command())
					if err != nil {
						t.Fatal(err)
					}
				}
				if x.Position.Turn != human || x.Dice == nil {
					t.Fatal("bot left session on another player's turn")
				}
				paths, err := game.LegalPaths(x.Position, *x.Dice)
				if err != nil {
					t.Fatal(err)
				}
				x, err = s.SetDraft(ctx, x.ID, app.DraftRequest{Command: command(), Prefix: paths[0].Steps})
				if err != nil {
					t.Fatal(err)
				}
				req := app.TurnRequest{Command: command(), Turn: game.Turn{Steps: x.Draft}}
				x, err = s.Confirm(ctx, x.ID, req)
				if err != nil {
					t.Fatal(err)
				}
				duplicate, err := s.Confirm(ctx, x.ID, req)
				if err != nil || !reflect.DeepEqual(x, duplicate) {
					t.Fatal("repeat applied a turn or bot action twice")
				}
			}
			if x.Phase != app.Finished || x.Outcome == nil {
				t.Fatal("game did not finish")
			}
			r, err := s.Replay(ctx, x.ID)
			if err != nil {
				t.Fatal(err)
			}
			final, err := replay.Play(r)
			if err != nil || final != x.Position {
				t.Fatal("history does not reproduce final position")
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, s = service(t, dir)
			defer store.Close()
			y, err := s.Get(ctx, x.ID)
			if err != nil || !reflect.DeepEqual(x, y) {
				t.Fatal("finished game lost after restart")
			}
			list, err := s.List(ctx)
			if err != nil || len(list) != 1 || list[0].ID != x.ID {
				t.Fatal("game history missing")
			}
		})
	}
}

func TestOpeningDiceCannotBeReplacedAndInvalidActionsDoNotPersist(t *testing.T) {
	store, s := service(t, t.TempDir())
	defer store.Close()
	ctx := context.Background()
	x, err := s.Create(ctx, app.CreateRequest{Command: app.Command{CommandID: "probe"}, Human: game.White, BotID: library.HeuristicID})
	if err != nil {
		t.Fatal(err)
	}
	starter := game.White
	last := x.Opening[len(x.Opening)-1]
	if last[1] > last[0] {
		starter = game.Black
	}
	x, err = s.Create(ctx, app.CreateRequest{Command: app.Command{CommandID: "opening-human"}, Human: starter, BotID: library.HeuristicID})
	if err != nil {
		t.Fatal(err)
	}
	if x.Phase != app.Moving || x.Dice == nil || *x.Dice != last || len(x.History) != 0 {
		t.Fatal("opening roll lost")
	}
	if _, err := s.Confirm(ctx, x.ID, app.TurnRequest{Command: app.Command{CommandID: "invalid", ExpectedVersion: x.Version}, Turn: game.Turn{Steps: []game.Step{{From: 0, To: 0, Die: 6}}}}); err == nil {
		t.Fatal("illegal turn accepted")
	}
	y, err := s.Get(ctx, x.ID)
	if err != nil || !reflect.DeepEqual(x, y) {
		t.Fatal("failed command mutated the game")
	}
}
