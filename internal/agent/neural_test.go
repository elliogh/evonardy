package agent_test

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"evonardy/internal/agent"
	"evonardy/internal/encoder"
	"evonardy/internal/game"
	"evonardy/internal/neural"
)

func TestNeuralWhiteMaximizesBlackMinimizesAndTiesStayFirst(t *testing.T) {
	parameters := make([]float64, neural.ParameterCount)
	for i := 0; i < 56; i++ {
		parameters[i] = math.Pow(float64(i%24+1), 2) / 1000
	}
	parameters[1824] = .8
	bot, err := agent.NewNeural(parameters)
	if err != nil {
		t.Fatal(err)
	}
	for side := game.White; side <= game.Black; side++ {
		p := game.Initial(side)
		p.FirstDone = [2]bool{true, true}
		p.Checkers[side][game.Head(side)] = 13
		p.Checkers[side][game.PhysicalPoint(side, 2)] = 1
		p.Checkers[side][game.PhysicalPoint(side, 5)] = 1
		dice := game.Dice{1, 2}
		actions, err := game.LegalActions(p, dice)
		if err != nil || len(actions) < 2 {
			t.Fatalf("need distinct legal actions: %v", err)
		}
		best, score, lowest, highest := 0, math.Inf(-1), math.Inf(1), math.Inf(-1)
		for i, action := range actions {
			input, err := encoder.Encode(action.Next)
			if err != nil {
				t.Fatal(err)
			}
			z := 0.0
			for j, value := range input {
				z += parameters[j] * value
			}
			value := math.Tanh(.8 * math.Tanh(z))
			lowest, highest = math.Min(lowest, value), math.Max(highest, value)
			if side == game.Black {
				value = -value
			}
			if value > score {
				best, score = i, value
			}
		}
		if highest-lowest < 1e-6 {
			t.Fatal("fixture must distinguish successor values")
		}
		got, evaluations, err := bot.ChooseMeasured(context.Background(), p, dice, actions)
		if err != nil || got != best || evaluations != len(actions) {
			t.Fatalf("side %d: want %d, got %d, evaluations %d, %v", side, best, got, evaluations, err)
		}
		zero, _ := agent.NewNeural(make([]float64, neural.ParameterCount))
		if got, err := zero.Choose(context.Background(), p, dice, actions); err != nil || got != 0 {
			t.Fatal("equal-valued successors must use stable first tie")
		}
		// Known dice only generate legal actions; inference does not read them.
		if got, err := bot.Choose(context.Background(), p, game.Dice{6, 6}, actions); err != nil || got != best {
			t.Fatal("dice leaked into neural ranking")
		}
	}
}

func TestNeuralTerminalRewardsOverrideAdversarialNetwork(t *testing.T) {
	for side := game.White; side <= game.Black; side++ {
		parameters := make([]float64, neural.ParameterCount)
		parameters[1856] = -100
		if side == game.Black {
			parameters[1856] = 100
		}
		bot, err := agent.NewNeural(parameters)
		if err != nil {
			t.Fatal(err)
		}
		p := game.Initial(side)
		p.Checkers[side] = [24]int{}
		p.Checkers[side][game.PhysicalPoint(side, 21)] = 1
		p.BorneOff[side] = 14
		p.FirstDone = [2]bool{true, true}
		dice := game.Dice{1, 3}
		actions, err := game.LegalActions(p, dice)
		if err != nil {
			t.Fatal(err)
		}
		choice, evaluations, err := bot.ChooseMeasured(context.Background(), p, dice, actions)
		if err != nil || evaluations != 0 {
			t.Fatalf("immediate victory must not require network evaluation: %v", err)
		}
		result, terminal := game.Result(actions[choice].Next)
		if !terminal || result.Winner != side {
			t.Fatal("missed exact immediate win")
		}
		value, err := bot.Value(actions[choice].Next)
		want := 1.0
		if side == game.Black {
			want = -1
		}
		if err != nil || value != want {
			t.Fatalf("terminal reward: want %g, got %g, %v", want, value, err)
		}
	}
}

func TestNeuralInferenceErrorsAndOwnedMemory(t *testing.T) {
	parameters := neural.Initialize(42, 0).Parameters()
	bot, err := agent.NewNeural(parameters)
	if err != nil {
		t.Fatal(err)
	}
	p := game.Initial(game.White)
	want, err := bot.Value(p)
	if err != nil {
		t.Fatal(err)
	}
	for i := range parameters {
		parameters[i] = 1e6
	}
	if got, err := bot.Value(p); err != nil || got != want {
		t.Fatal("learner parameter edits changed frozen inference")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := bot.Choose(ctx, p, game.Dice{}, nil); err == nil {
		t.Fatal("ignored cancellation")
	}
	if _, err := bot.Choose(context.Background(), p, game.Dice{}, nil); err == nil {
		t.Fatal("accepted missing legal actions")
	}
	p.Ruleset = "invalid"
	if _, err := bot.Value(p); err == nil {
		t.Fatal("accepted invalid public position")
	}
	if _, err := agent.NewNeural([]float64{1}); err == nil {
		t.Fatal("accepted invalid network shape")
	}
}

func TestNeuralFrozenPolicyJSONAndCompatibility(t *testing.T) {
	p := agent.Policy{ID: "snapshot", Kind: "neural", EncoderVersion: encoder.Version, NetworkVersion: neural.Version}
	copy(p.NeuralParameters[:], neural.Initialize(42, 0).Parameters())
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var restored agent.Policy
	if err := json.Unmarshal(data, &restored); err != nil || restored != p {
		t.Fatalf("policy lost value semantics: %v", err)
	}
	frozen := p.New(nil)
	if frozen == nil {
		t.Fatal("valid neural policy must construct without an RNG")
	}
	p.NeuralParameters[0] += 3
	if restored.NeuralParameters[0] == p.NeuralParameters[0] {
		t.Fatal("policy copies alias parameters")
	}
	for _, mutate := range []func(*agent.Policy){
		func(p *agent.Policy) { p.EncoderVersion = "unsupported" },
		func(p *agent.Policy) { p.NetworkVersion = "unsupported" },
		func(p *agent.Policy) { p.Weights[0] = 1 },
		func(p *agent.Policy) { p.NeuralParameters[0] = math.NaN() },
		func(p *agent.Policy) { p.NeuralParameters[0] = 1e7 },
	} {
		bad := restored
		mutate(&bad)
		if bad.Validate() == nil || bad.New(nil) != nil {
			t.Fatal("accepted incompatible neural policy")
		}
	}
	for _, raw := range []string{`null`, `[]`, `[1]`, `[1e999]`} {
		var params agent.NeuralParameters
		if err := json.Unmarshal([]byte(raw), &params); err == nil {
			t.Fatalf("accepted malformed parameter shape %s", raw)
		}
	}
	legacy := agent.Policy{ID: "legacy", Kind: "linear"}
	data, err = json.Marshal(legacy)
	if err != nil || strings.Contains(string(data), "neural") || strings.Contains(string(data), "encoder_version") || strings.Contains(string(data), "network_version") {
		t.Fatal("new fields changed legacy policy JSON")
	}
	legacy.NetworkVersion = neural.Version
	if legacy.Validate() == nil {
		t.Fatal("non-neural policy accepted neural metadata")
	}
}
