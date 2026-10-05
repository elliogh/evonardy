package game_test

import (
	"evonardy/internal/game"
	"testing"
)

func TestLegalActionsIncludeValidatedSuccessors(t *testing.T) {
	p := game.Initial(game.White)
	dice := game.Dice{2, 3}
	actions, err := game.LegalActions(p, dice)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range actions {
		next, err := game.ApplyTurn(p, dice, action.Turn)
		if err != nil {
			t.Fatal(err)
		}
		if next != action.Next {
			t.Fatal("successor differs from public application")
		}
	}
}
