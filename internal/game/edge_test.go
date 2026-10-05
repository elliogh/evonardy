package game_test

import (
	"slices"
	"testing"

	"evonardy/internal/game"
)

func physicalPosition(own, opponent map[int]int) game.Position {
	p := game.Initial(game.White)
	p.Checkers = [2][24]int{}
	p.BorneOff = [2]int{15, 15}
	p.FirstDone = [2]bool{true, true}
	for i, pieces := range []map[int]int{own, opponent} {
		for point, count := range pieces {
			p.Checkers[i][point] = count
			p.BorneOff[i] -= count
		}
	}
	return p
}

func rotateSwap(p game.Position) game.Position {
	q := p
	q.Turn = p.Turn.Other()
	q.Starter = p.Starter.Other()
	q.BorneOff = [2]int{p.BorneOff[1], p.BorneOff[0]}
	q.FirstDone = [2]bool{p.FirstDone[1], p.FirstDone[0]}
	for player := game.White; player <= game.Black; player++ {
		for point, count := range p.Checkers[player] {
			q.Checkers[player.Other()][(point+12)%24] = count
		}
	}
	return q
}

func rotateTurn(turn game.Turn) game.Turn {
	result := game.Turn{Steps: append([]game.Step{}, turn.Steps...)}
	for i := range result.Steps {
		result.Steps[i].From = (result.Steps[i].From + 12) % 24
		if result.Steps[i].To != game.Off {
			result.Steps[i].To = (result.Steps[i].To + 12) % 24
		}
	}
	return result
}

func TestBlockOnFiniteOpponentRoute(t *testing.T) {
	cases := []struct {
		name  string
		p     game.Position
		valid bool
	}{
		{"crosses_physical_zero", physicalPosition(map[int]int{22: 1, 23: 1, 0: 10, 1: 1, 2: 1, 3: 1}, map[int]int{20: 15}), false},
		{"route_endpoints_not_joined", physicalPosition(map[int]int{10: 10, 11: 1, 12: 1, 13: 1, 14: 1, 15: 1}, map[int]int{16: 15}), true},
		{"one_opponent_ahead", physicalPosition(map[int]int{15: 10, 16: 1, 17: 1, 18: 1, 19: 1, 20: 1}, map[int]int{12: 14, 21: 1}), true},
		{"one_opponent_off", physicalPosition(map[int]int{15: 10, 16: 1, 17: 1, 18: 1, 19: 1, 20: 1}, map[int]int{12: 14}), true},
	}
	for _, tc := range cases {
		for _, p := range []game.Position{tc.p, rotateSwap(tc.p)} {
			if valid := game.ValidatePosition(p) == nil; valid != tc.valid {
				t.Fatalf("%s: valid=%v", tc.name, valid)
			}
		}
	}
}

func TestIntermediateBlockRejectedEvenWhenFinalWouldBeLegal(t *testing.T) {
	p := physicalPosition(map[int]int{14: 10, 15: 1, 16: 1, 17: 1, 19: 1, 20: 1}, map[int]int{12: 15})
	dice := game.Dice{4, 1}
	illegal := game.Turn{Steps: []game.Step{{From: 14, To: 18, Die: 4}, {From: 18, To: 19, Die: 1}}}
	for _, rotated := range []bool{false, true} {
		state, turn := p, illegal
		if rotated {
			state = rotateSwap(p)
			turn = rotateTurn(illegal)
		}
		if err := game.ValidatePosition(state); err != nil {
			t.Fatal(err)
		}
		if _, err := game.ApplyTurn(state, dice, turn); err == nil {
			t.Fatal("accepted illegal intermediate block")
		}
		if _, err := game.LegalContinuations(state, dice, turn.Steps[:1]); err == nil {
			t.Fatal("UI offered illegal prefix")
		}
		final := state
		for _, step := range turn.Steps {
			final.Checkers[state.Turn][step.From]--
			final.Checkers[state.Turn][step.To]++
		}
		if err := game.ValidatePosition(final); err != nil {
			t.Fatalf("fixture final should be legal: %v", err)
		}
	}
}

func TestBearOffAndMandatoryMovesRejectInvalidActions(t *testing.T) {
	cases := []struct {
		f     fixture
		steps []game.Step
	}{
		{fixture{Own: map[string]int{"20": 1, "22": 1}, Opponent: map[string]int{"0": 15}, Dice: game.Dice{6, 6}}, []game.Step{{From: 22, To: game.Off, Die: 6}}},
		{fixture{Own: map[string]int{"17": 1, "23": 1}, Opponent: map[string]int{"0": 15}, Dice: game.Dice{2, 2}}, []game.Step{{From: 23, To: game.Off, Die: 2}}},
		{fixture{Own: map[string]int{"23": 1}, Opponent: map[string]int{"0": 15}, Dice: game.Dice{1, 2}}, []game.Step{{From: 23, To: game.Off, Die: 1}}},
	}
	for _, tc := range cases {
		p := fixturePosition(t, tc.f, game.White)
		if _, err := game.ApplyTurn(p, tc.f.Dice, game.Turn{Steps: tc.steps}); err == nil {
			t.Fatalf("accepted %+v", tc.steps)
		}
	}
}

func TestPassTerminalAndMars(t *testing.T) {
	f := fixture{Own: map[string]int{"0": 15}, Opponent: map[string]int{"0": 13, "13": 1, "14": 1}, Dice: game.Dice{1, 2}, SecondFirst: true}
	p := fixturePosition(t, f, game.White)
	turns, err := game.LegalTurns(p, f.Dice)
	if err != nil || len(turns) != 1 || len(turns[0].Steps) != 0 {
		t.Fatalf("pass: %+v %v", turns, err)
	}
	q, err := game.ApplyTurn(p, f.Dice, turns[0])
	if err != nil || !q.FirstDone[0] || q.Turn != game.Black {
		t.Fatalf("pass transition: %+v %v", q, err)
	}
	for _, opponentOff := range []int{0, 1} {
		p := physicalPosition(map[int]int{23: 1}, map[int]int{12: 15 - opponentOff})
		q, err := game.ApplyTurn(p, game.Dice{6, 6}, game.Turn{Steps: []game.Step{{From: 23, To: 24, Die: 6}}})
		if err != nil {
			t.Fatal(err)
		}
		r, ok := game.Result(q)
		if !ok || r.Winner != game.White || r.Mars != (opponentOff == 0) || r.Points != 2-opponentOff {
			t.Fatalf("result: %+v", r)
		}
		if legal, err := game.LegalTurns(q, game.Dice{1, 2}); err != nil || len(legal) != 0 {
			t.Fatal("moves after victory")
		}
		if _, err := game.ApplyTurn(q, game.Dice{1, 2}, game.Turn{}); err == nil {
			t.Fatal("pass after victory")
		}
	}
}

func TestColorRotationSymmetry(t *testing.T) {
	for _, f := range fixtures(t) {
		p := fixturePosition(t, f, game.White)
		q := rotateSwap(p)
		paths, err := game.LegalPaths(p, f.Dice)
		if err != nil {
			t.Fatal(err)
		}
		other, err := game.LegalPaths(q, f.Dice)
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) != len(other) {
			t.Fatalf("%s: path counts differ", f.Name)
		}
		for _, turn := range paths {
			rotated := rotateTurn(turn)
			if !slices.ContainsFunc(other, func(x game.Turn) bool { return slices.Equal(x.Steps, rotated.Steps) }) {
				t.Fatalf("%s: rotated path missing", f.Name)
			}
		}
	}
}

func TestInvalidDiceAndPrefix(t *testing.T) {
	p := game.Initial(game.White)
	for _, dice := range []game.Dice{{0, 1}, {1, 7}, {-1, 4}} {
		if _, err := game.LegalTurns(p, dice); err == nil {
			t.Fatal("invalid dice")
		}
	}
	if _, err := game.LegalContinuations(p, game.Dice{1, 2}, []game.Step{{From: 0, To: 3, Die: 3}}); err == nil {
		t.Fatal("invalid prefix")
	}
}

func TestHomeMovementAndLastEntryCanBeChosen(t *testing.T) {
	p := physicalPosition(map[int]int{18: 1, 23: 1}, map[int]int{12: 15})
	dice := game.Dice{1, 6}
	for _, turn := range []game.Turn{
		{Steps: []game.Step{{From: 23, To: game.Off, Die: 1}, {From: 18, To: game.Off, Die: 6}}},
		{Steps: []game.Step{{From: 18, To: 19, Die: 1}, {From: 19, To: game.Off, Die: 6}}},
	} {
		if _, err := game.ApplyTurn(p, dice, turn); err != nil {
			t.Fatal(err)
		}
	}
	p = physicalPosition(map[int]int{17: 1, 23: 1}, map[int]int{12: 15})
	q, err := game.ApplyTurn(p, dice, game.Turn{Steps: []game.Step{{From: 17, To: 18, Die: 1}, {From: 18, To: game.Off, Die: 6}}})
	if err != nil || q.BorneOff[0] != 14 {
		t.Fatalf("entry then bear-off: %+v %v", q, err)
	}
}
