package replay_test

import (
	"evonardy/internal/game"
	"evonardy/internal/replay"
	"testing"
)

func TestUnfinishedHistoryReconstructsWithoutInventingAResult(t *testing.T) {
	p := game.Initial(game.White)
	d := game.Dice{6, 2}
	r := replay.New(p, []game.Dice{d}, [2]string{"human", "heuristic"}, 42, 0, 100)
	positions, err := replay.Positions(r)
	if err != nil || len(positions) != 1 || positions[0] != p {
		t.Fatal("pending opening history rejected")
	}
	if _, err := replay.Play(r); err == nil {
		t.Fatal("unfinished history presented as a completed replay")
	}
	turns, err := game.LegalTurns(p, d)
	if err != nil {
		t.Fatal(err)
	}
	after, err := game.ApplyTurn(p, d, turns[0])
	if err != nil {
		t.Fatal(err)
	}
	r.Append(d, turns[0], after)
	positions, err = replay.Positions(r)
	if err != nil || len(positions) != 2 || positions[1] != after {
		t.Fatal("unfinished history lost a full turn")
	}
	r.Events[0].AfterHash = "corrupt"
	if _, err := replay.Positions(r); err == nil {
		t.Fatal("corrupted history accepted")
	}
}
