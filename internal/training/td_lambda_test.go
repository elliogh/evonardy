package training_test

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"slices"
	"testing"

	"evonardy/internal/encoder"
	"evonardy/internal/game"
	"evonardy/internal/neural"
	"evonardy/internal/training"
)

func TestAccumulatingTracesMatchIndependentTrajectory(t *testing.T) {
	positions := tdTrajectory(t)
	parameters := make([]float64, neural.ParameterCount)
	parameters[0], parameters[1792], parameters[1824], parameters[1856] = .4, .1, .7, -.2
	learner, err := training.NewTDLambda(parameters, .01, .7)
	if err != nil {
		t.Fatal(err)
	}
	wantParameters := slices.Clone(parameters)
	var wantTraces neural.Gradient
	for step := 0; step < len(positions)-1; step++ {
		input, err := encoder.Encode(positions[step])
		if err != nil {
			t.Fatal(err)
		}
		nextInput, err := encoder.Encode(positions[step+1])
		if err != nil {
			t.Fatal(err)
		}
		// Independently evaluate the single active hidden neuron. All others
		// remain exactly zero throughout this artificial trajectory.
		value, h := oneNeuronValue(wantParameters, input)
		q := 1 - value*value
		r := q * wantParameters[1824] * (1 - h*h)
		var gradient neural.Gradient
		for i, x := range input {
			gradient[i] = r * x
		}
		gradient[1792], gradient[1824], gradient[1856] = r, q*h, q
		target, _ := oneNeuronValue(wantParameters, nextInput)
		_, terminal := game.Result(positions[step+1])
		if terminal {
			target = 1
		}
		for i := range wantTraces {
			wantTraces[i] = .7*wantTraces[i] + gradient[i]
			wantParameters[i] += .01 * (target - value) * wantTraces[i]
		}
		if _, err := learner.Update(positions[step], positions[step+1]); err != nil {
			t.Fatal(err)
		}
		gotParameters := learner.Parameters()
		for i, want := range wantParameters {
			if math.Abs(gotParameters[i]-want) > 1e-14 {
				t.Fatalf("step %d parameter %d: want %.17g, got %.17g", step, i, want, gotParameters[i])
			}
		}
		if terminal {
			wantTraces = neural.Gradient{}
		}
		gotTraces := learner.Traces()
		for i, want := range wantTraces {
			if math.Abs(gotTraces[i]-want) > 1e-14 {
				t.Fatalf("step %d trace %d: want %.17g, got %.17g", step, i, want, gotTraces[i])
			}
		}
	}
	if learner.Traces() != (neural.Gradient{}) {
		t.Fatal("terminal transition leaked traces into the next game")
	}
}

func TestLambdaZeroAgreesExactlyWithTDZero(t *testing.T) {
	parameters := neural.Initialize(42, 0).Parameters()
	a, err := training.NewTDLearner(parameters, .001)
	if err != nil {
		t.Fatal(err)
	}
	b, err := training.NewTDLambda(parameters, .001, 0)
	if err != nil {
		t.Fatal(err)
	}
	positions := tdTrajectory(t)
	for i := 0; i < len(positions)-1; i++ {
		first, err := a.Update(positions[i], positions[i+1])
		if err != nil {
			t.Fatal(err)
		}
		second, err := b.Update(positions[i], positions[i+1])
		if err != nil || first != second || !slices.Equal(a.Parameters(), b.Parameters()) {
			t.Fatal("lambda=0 changed TD(0) updates or accounting")
		}
	}
}

func TestTraceResetReplacementAndGameBoundaryRestore(t *testing.T) {
	positions := tdTrajectory(t)
	l, err := training.NewTDLambda(neural.Initialize(42, 0).Parameters(), .001, .7)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Update(positions[0], positions[1]); err != nil {
		t.Fatal(err)
	}
	if l.Traces() == (neural.Gradient{}) {
		t.Fatal("nonterminal update failed to accumulate traces")
	}
	beforeParameters, beforeTraces := l.Parameters(), l.Traces()
	bad := positions[1]
	bad.Ruleset = "invalid"
	if _, err := l.Update(positions[1], bad); err == nil || l.Traces() != beforeTraces || !slices.Equal(l.Parameters(), beforeParameters) {
		t.Fatal("failed update changed weights or traces")
	}
	replacement, err := training.NewTDLambda(l.Parameters(), .001, .7)
	if err != nil || replacement.Traces() != (neural.Gradient{}) {
		t.Fatal("copied participant inherited parent traces")
	}
	l.ResetTraces()
	if l.Traces() != (neural.Gradient{}) || !slices.Equal(l.Parameters(), beforeParameters) {
		t.Fatal("trace reset changed weights")
	}
	c := training.DefaultTDLambdaConfig()
	c.Games, c.MaxTurns = 2, 4
	s, err := training.NewTD(c)
	if err != nil || s.Algorithm != training.TDLambdaAlgorithm {
		t.Fatal("missing separate lambda experiment contract")
	}
	first, _, err := training.TrainTDGame(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var restored training.TDState
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	continuous, continuousReplay, err := training.TrainTDGame(context.Background(), first)
	if err != nil {
		t.Fatal(err)
	}
	resumed, resumedReplay, err := training.TrainTDGame(context.Background(), restored)
	if err != nil || !reflect.DeepEqual(continuous, resumed) || !reflect.DeepEqual(continuousReplay, resumedReplay) {
		t.Fatal("traces leaked across a truncated/resumed game boundary")
	}
}

func TestRejectInvalidLambda(t *testing.T) {
	for _, lambda := range []float64{-1, 2, math.NaN(), math.Inf(1)} {
		if _, err := training.NewTDLambda(make([]float64, neural.ParameterCount), .001, lambda); err == nil {
			t.Fatal("accepted invalid trace decay")
		}
		c := training.DefaultTDConfig()
		c.Lambda = lambda
		if _, err := training.NewTD(c); err == nil {
			t.Fatal("accepted invalid lambda config")
		}
	}
	large := make([]float64, neural.ParameterCount)
	large[0], large[1792], large[1824] = .5, -.5, 1e6
	l, err := training.NewTDLambda(large, 1, .7)
	if err != nil {
		t.Fatal(err)
	}
	p := game.Initial(game.White)
	next := p
	next.Turn = game.Black
	if _, err := l.Update(p, next); err != nil {
		t.Fatal(err)
	}
	traces := l.Traces()
	blackWin := next
	blackWin.Checkers[game.Black], blackWin.BorneOff[game.Black] = [24]int{}, 15
	if _, err := l.Update(p, blackWin); err == nil || l.Traces() != traces || !slices.Equal(l.Parameters(), large) {
		t.Fatal("out-of-bounds update partially committed accumulating traces")
	}
}

func oneNeuronValue(p []float64, input encoder.Vector) (float64, float64) {
	z := p[1792]
	for i, x := range input {
		z += p[i] * x
	}
	h := math.Tanh(z)
	return math.Tanh(p[1856] + p[1824]*h), h
}

func tdTrajectory(t *testing.T) []game.Position {
	t.Helper()
	p := game.Initial(game.White)
	a, err := game.ApplyTurn(p, game.Dice{1, 2}, game.Turn{Steps: []game.Step{{From: 0, To: 1, Die: 1}, {From: 1, To: 3, Die: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := game.ApplyTurn(a, game.Dice{1, 2}, game.Turn{Steps: []game.Step{{From: 12, To: 13, Die: 1}, {From: 13, To: 15, Die: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	terminal := b
	terminal.Checkers[game.White], terminal.BorneOff[game.White], terminal.Turn = [24]int{}, 15, game.Black
	return []game.Position{p, a, b, terminal}
}
