package training_test

import (
	"context"
	"math"
	"reflect"
	"slices"
	"testing"

	"evonardy/internal/game"
	"evonardy/internal/neural"
	"evonardy/internal/replay"
	"evonardy/internal/training"
)

func TestTDWhiteRewardAndStopGradientBootstrap(t *testing.T) {
	current := game.Initial(game.White)
	current.FirstDone = [2]bool{true, true}
	whiteWin, blackWin, nonterminal := current, current, current
	whiteWin.Checkers[game.White], whiteWin.BorneOff[game.White], whiteWin.Turn = [24]int{}, 15, game.Black
	blackWin.Checkers[game.Black], blackWin.BorneOff[game.Black], blackWin.Turn = [24]int{}, 15, game.White
	nonterminal.Checkers[game.White][0], nonterminal.Checkers[game.White][1], nonterminal.Turn = 14, 1, game.Black
	for _, test := range []struct {
		name   string
		next   game.Position
		turn   game.Player
		target float64
	}{
		{"White terminal target", whiteWin, game.White, 1},
		{"Black terminal target", blackWin, game.Black, -1},
		{"nonterminal bootstrap", nonterminal, game.White, .10819685859445924},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := make([]float64, neural.ParameterCount)
			p[0], p[1792], p[1824], p[1856] = .4, .1, .7, -.2
			before := slices.Clone(p)
			learner, err := training.NewTDLearner(p, .001)
			if err != nil {
				t.Fatal(err)
			}
			position := current
			position.Turn = test.turn
			update, err := learner.Update(position, test.next)
			if err != nil {
				t.Fatal(err)
			}
			// Closed-form derivatives at x[0]=1: h=tanh(.5),
			// v=tanh(.7*h-.2), q=1-v*v, r=q*.7*(1-h*h).
			// Only the current state's value is differentiated, including
			// its active metadata inputs. Next-state derivatives are absent.
			h := math.Tanh(.5)
			value := math.Tanh(.7*h - .2)
			q, scale := 1-value*value, .001*(test.target-value)
			r := q * .7 * (1 - h*h)
			want := slices.Clone(p)
			for _, i := range []int{0, 36, 50 + int(test.turn), 54, 1792} {
				want[i] += scale * r
			}
			want[1824] += scale * q * h
			want[1856] += scale * q
			for i, got := range learner.Parameters() {
				if math.Abs(got-want[i]) > 1e-14 {
					t.Fatalf("parameter %d: want %.17g, got %.17g", i, want[i], got)
				}
			}
			terminal := test.name != "nonterminal bootstrap"
			forwards := 2
			if terminal {
				forwards = 1
			}
			if math.Abs(update.Value-value) > 1e-14 || math.Abs(update.Target-test.target) > 1e-14 || update.Terminal != terminal || update.ForwardEvaluations != forwards {
				t.Fatalf("incorrect target/sign/forward contract: %+v", update)
			}
			if !slices.Equal(p, before) {
				t.Fatal("parameters changed")
			}
		})
	}
}

func TestTDRealSelfPlayIsFiniteBoundedAndDiceIndependent(t *testing.T) {
	var records [2]replay.Record
	for i, epsilon := range []float64{0, 1} {
		config := training.DefaultTDConfig()
		config.Games, config.MaxTurns, config.Epsilon = 1, 8, epsilon
		s, err := training.NewTD(config)
		if err != nil {
			t.Fatal(err)
		}
		before := s
		next, played, err := training.TrainTDGame(context.Background(), s)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(s, before) || next.Parameters == s.Parameters || next.Counters.Games != 1 || next.Counters.Updates != 8 || next.Counters.Decisions != 8 || next.Counters.CompletedGames != 0 || next.Counters.TruncatedGames != 1 {
			t.Fatalf("real game must update an independent state without inventing a winner: %+v", next.Counters)
		}
		if err := training.ValidateTD(next); err != nil {
			t.Fatal(err)
		}
		if _, err := replay.Play(played.Replay); err != nil || played.Replay.Status != replay.Truncated || played.Replay.Outcome != nil {
			t.Fatalf("actual self-play replay must verify as truncated: %v", err)
		}
		for _, parameter := range next.Parameters {
			if math.IsNaN(parameter) || math.IsInf(parameter, 0) {
				t.Fatal("real self-play produced nonfinite parameters")
			}
		}
		wantForwards := uint64(16) // current value/gradient and next bootstrap per turn
		if epsilon == 0 {
			positions, err := replay.Positions(played.Replay)
			if err != nil {
				t.Fatal(err)
			}
			for j, event := range played.Replay.Events {
				actions, err := game.LegalActions(positions[j], event.Dice)
				if err != nil {
					t.Fatal(err)
				}
				wantForwards += uint64(len(actions))
			}
		}
		if next.Counters.ForwardEvaluations != wantForwards {
			t.Fatalf("want %d actual forwards, got %d", wantForwards, next.Counters.ForwardEvaluations)
		}
		records[i] = played.Replay
		if _, _, err := training.TrainTDGame(context.Background(), next); err == nil {
			t.Fatal("exceeded configured game budget")
		}
	}
	if !reflect.DeepEqual(records[0].Opening, records[1].Opening) {
		t.Fatal("exploration changed opening dice")
	}
	for i := range records[0].Events {
		if records[0].Events[i].Dice != records[1].Events[i].Dice {
			t.Fatal("exploration changed environment dice")
		}
	}
}

func TestTDRejectsInvalidInputsAndKeepsStateOnCancellation(t *testing.T) {
	for _, alpha := range []float64{0, -1, 2, math.NaN(), math.Inf(1)} {
		if _, err := training.NewTDLearner(make([]float64, neural.ParameterCount), alpha); err == nil {
			t.Fatal("accepted invalid alpha")
		}
	}
	if _, err := training.NewTDLearner([]float64{1}, .001); err == nil {
		t.Fatal("accepted invalid network shape")
	}
	parameters := neural.Initialize(42, 0).Parameters()
	l, err := training.NewTDLearner(parameters, .001)
	if err != nil {
		t.Fatal(err)
	}
	p := game.Initial(game.White)
	bad := p
	bad.Ruleset = "invalid"
	terminal := p
	terminal.Checkers[game.White], terminal.BorneOff[game.White] = [24]int{}, 15
	for _, transition := range [][2]game.Position{{bad, p}, {p, bad}, {terminal, p}} {
		if _, err := l.Update(transition[0], transition[1]); err == nil || !slices.Equal(parameters, l.Parameters()) {
			t.Fatal("invalid update changed parameters")
		}
	}
	// Force a later parameter to exceed the publication bound after earlier
	// parameters have already been calculated, proving atomic update failure.
	large := make([]float64, neural.ParameterCount)
	large[0], large[1792], large[1824] = .5, -.5, 1e6
	guarded, err := training.NewTDLearner(large, 1)
	if err != nil {
		t.Fatal(err)
	}
	blackWin := p
	blackWin.Checkers[game.Black], blackWin.BorneOff[game.Black] = [24]int{}, 15
	if _, err := guarded.Update(p, blackWin); err == nil || !slices.Equal(large, guarded.Parameters()) {
		t.Fatal("out-of-bounds update partially mutated the network")
	}
	c := training.DefaultTDConfig()
	c.Games, c.MaxTurns = 2, 2
	s, err := training.NewTD(c)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	got, _, err := training.TrainTDGame(ctx, s)
	if err == nil || !reflect.DeepEqual(got, s) {
		t.Fatal("canceled game committed partial training state")
	}
	for _, mutate := range []func(*training.TDConfig){
		func(c *training.TDConfig) { c.Games = 0 },
		func(c *training.TDConfig) { c.Games = 200001 },
		func(c *training.TDConfig) { c.MaxTurns = 0 },
		func(c *training.TDConfig) { c.MaxTurns = 10001 },
		func(c *training.TDConfig) { c.Games, c.MaxTurns = 200000, 10000 },
		func(c *training.TDConfig) { c.Alpha = math.NaN() },
		func(c *training.TDConfig) { c.Epsilon = math.Inf(1) },
		func(c *training.TDConfig) { c.Epsilon = -1 },
		func(c *training.TDConfig) { c.Epsilon = 2 },
	} {
		bad := c
		mutate(&bad)
		if _, err := training.NewTD(bad); err == nil {
			t.Fatal("accepted invalid TD config")
		}
	}
	for _, mutate := range []func(*training.TDState){
		func(s *training.TDState) { s.Version++ },
		func(s *training.TDState) { s.Algorithm = "unsupported" },
		func(s *training.TDState) { s.EncoderVersion = "unsupported" },
		func(s *training.TDState) { s.Parameters[0] = math.NaN() },
		func(s *training.TDState) { s.Counters.Games++ },
		func(s *training.TDState) { s.Games++ },
	} {
		bad := s
		mutate(&bad)
		if _, _, err := training.TrainTDGame(context.Background(), bad); err == nil {
			t.Fatal("accepted invalid TD state")
		}
	}
}

func TestTDFinishedSelfPlayRecordsActualTerminalOutcome(t *testing.T) {
	c := training.DefaultTDConfig()
	c.Games, c.Epsilon = 1, 1
	s, err := training.NewTD(c)
	if err != nil {
		t.Fatal(err)
	}
	next, played, err := training.TrainTDGame(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if played.Replay.Status != replay.Completed || played.Replay.Outcome == nil || next.Counters.CompletedGames != 1 || next.Counters.TruncatedGames != 0 {
		t.Fatalf("fixed random-exploration fixture must complete: %+v", next.Counters)
	}
	if _, err := replay.Play(played.Replay); err != nil {
		t.Fatal(err)
	}
	if next.Counters.ForwardEvaluations != 2*next.Counters.Decisions-1 || next.Counters.Updates != next.Counters.Decisions {
		t.Fatal("terminal self-play step bootstrapped or failed to update")
	}
	if err := training.ValidateTD(next); err != nil {
		t.Fatal(err)
	}
}

func TestGAAccountingDoesNotInventGradientUpdates(t *testing.T) {
	s, err := training.New(training.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if err := training.Validate(s); err != nil {
		t.Fatal(err)
	}
	s.Counters.Updates = 1
	if training.Validate(s) == nil {
		t.Fatal("GA-linear checkpoint cannot claim neural gradient updates")
	}
}
