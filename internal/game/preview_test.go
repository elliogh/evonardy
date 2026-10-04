package game_test

import (
	"evonardy/internal/game"
	"testing"
)

func TestPreviewPreservesTurnBoundaryAndValidatesPrefixes(t *testing.T) {
	p := game.Initial(game.White)
	q, options, err := game.PreviewTurn(p, game.Dice{1, 2}, []game.Step{{From: 0, To: 1, Die: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if q.Checkers[0][0] != 14 || q.Checkers[0][1] != 1 || q.Turn != p.Turn || q.FirstDone != p.FirstDone || options.Complete {
		t.Fatal("preview changed turn boundary")
	}
	if _, _, err := game.PreviewTurn(p, game.Dice{1, 2}, []game.Step{{From: 0, To: 3, Die: 3}}); err == nil {
		t.Fatal("illegal preview accepted")
	}
}
