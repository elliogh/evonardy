package neural_test

import (
	"math"
	"slices"
	"sync"
	"testing"

	"evonardy/internal/encoder"
	"evonardy/internal/game"
	"evonardy/internal/neural"
	"evonardy/internal/random"
)

func TestContractAndKnownForwardValues(t *testing.T) {
	if neural.Version != "tanh-56-32-1-v1" || neural.Inputs != 56 || neural.Hidden != 32 || neural.Outputs != 1 || neural.ParameterCount != 1857 {
		t.Fatal("unexpected network contract")
	}
	sparse := make([]float64, 1857)
	sparse[0], sparse[55], sparse[1792] = .8, -.4, .2
	sparse[31*56], sparse[31*56+55], sparse[1792+31] = -.6, .2, -.15
	sparse[1824], sparse[1824+31], sparse[1856] = .7, -.3, .1
	for _, test := range []struct {
		name       string
		parameters []float64
		input      encoder.Vector
		want       float64
	}{
		{"zero", make([]float64, 1857), denseInput(), 0},
		// Hidden preactivations are .5 and -.4. Independently evaluated in
		// Python: tanh(.7*tanh(.5) - .3*tanh(-.4) + .1).
		{"sparse first and last neurons", sparse, encoder.Vector{0: .5, 55: .25}, .491067957943725},
		// A separate Python math calculation of the dense fixture, using
		// theta[i]=.035*sin((i+1)*.37), x[i]=.1+.8*(i+1)/56.
		{"all neurons", denseParameters(), denseInput(), .04554917885769026},
	} {
		t.Run(test.name, func(t *testing.T) {
			n := mustNetwork(t, test.parameters)
			before := test.input
			got, err := n.Forward(test.input)
			if err != nil || math.Abs(got-test.want) > 1e-14 {
				t.Fatalf("want %.17g, got %.17g, %v", test.want, got, err)
			}
			withGradient, _, err := n.ValueGradient(test.input)
			if err != nil || withGradient != got || test.input != before || !slices.Equal(test.parameters, n.Parameters()) {
				t.Fatal("gradient evaluation changed the value, input, or parameters")
			}
		})
	}
	var zero neural.Network
	if got, err := zero.Forward(denseInput()); err != nil || got != 0 {
		t.Fatal("zero network must be valid")
	}
}

func TestClosedFormGradient(t *testing.T) {
	p := make([]float64, 1857)
	p[0], p[1792], p[1824], p[1856] = .4, .1, .7, -.2
	n := mustNetwork(t, p)
	input := encoder.Vector{0: .5}
	value, got, err := n.ValueGradient(input)
	if err != nil {
		t.Fatal(err)
	}
	h := math.Tanh(.3)
	v := math.Tanh(.7*h - .2)
	d := 1 - v*v
	var want neural.Gradient
	want[0] = d * .7 * (1 - h*h) * .5
	want[1792] = d * .7 * (1 - h*h)
	want[1824] = d * h
	want[1856] = d
	if math.Abs(value-v) > 1e-14 {
		t.Fatal("incorrect value")
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-14 {
			t.Fatalf("gradient %d: want %g, got %g", i, want[i], got[i])
		}
	}
}

func TestEveryParameterGradientAgainstCentralDifferences(t *testing.T) {
	position, err := encoder.Encode(game.Initial(game.Black))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name       string
		parameters []float64
		input      encoder.Vector
	}{
		{"dense fixture", denseParameters(), denseInput()},
		{"initialized network and encoded position", neural.Initialize(42, 7).Parameters(), position},
	} {
		t.Run(test.name, func(t *testing.T) {
			n := mustNetwork(t, test.parameters)
			_, gradient, err := n.ValueGradient(test.input)
			if err != nil {
				t.Fatal(err)
			}
			const epsilon = 1e-6
			for i, analytical := range gradient {
				plus, minus := slices.Clone(test.parameters), slices.Clone(test.parameters)
				plus[i] += epsilon
				minus[i] -= epsilon
				nPlus, nMinus := mustNetwork(t, plus), mustNetwork(t, minus)
				vPlus, errPlus := nPlus.Forward(test.input)
				vMinus, errMinus := nMinus.Forward(test.input)
				if errPlus != nil || errMinus != nil {
					t.Fatalf("perturbation failed: %v, %v", errPlus, errMinus)
				}
				numerical := (vPlus - vMinus) / (2 * epsilon)
				tolerance := 1e-8 + 1e-5*math.Max(math.Abs(analytical), math.Abs(numerical))
				if math.IsNaN(analytical) || math.IsInf(analytical, 0) || math.Abs(analytical-numerical) > tolerance {
					t.Fatalf("parameter %d: analytical %.17g, numerical %.17g, tolerance %g", i, analytical, numerical, tolerance)
				}
			}
		})
	}
}

func TestInitializationUsesAnIndependentReproducibleStream(t *testing.T) {
	a := neural.Initialize(42, 7)
	dice, exploration := random.New(42, "dice", 7), random.New(42, "exploration", 7)
	for range 100 {
		dice.IntN(6)
		exploration.NormFloat64()
	}
	stateBefore, err := dice.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	b := neural.Initialize(42, 7)
	stateAfter, err := dice.MarshalBinary()
	if err != nil || !slices.Equal(stateBefore, stateAfter) || !slices.Equal(a.Parameters(), b.Parameters()) {
		t.Fatal("initialization must not consume or depend on dice/exploration streams")
	}
	if slices.Equal(a.Parameters(), neural.Initialize(43, 7).Parameters()) || slices.Equal(a.Parameters(), neural.Initialize(42, 8).Parameters()) {
		t.Fatal("seed and index must select different networks")
	}
	// Check the documented PCG domain and draw order, including zero biases.
	r := random.New(42, "neural/initialization/v1", 7)
	p := a.Parameters()
	for i := 0; i < 1792; i++ {
		if p[i] != r.NormFloat64()*math.Sqrt(2.0/88) {
			t.Fatalf("hidden initialization differs at %d", i)
		}
	}
	for i := 1792; i < 1824; i++ {
		if p[i] != 0 {
			t.Fatal("hidden bias must start at zero")
		}
	}
	for i := 1824; i < 1856; i++ {
		if p[i] != r.NormFloat64()*math.Sqrt(2.0/33) {
			t.Fatalf("output initialization differs at %d", i)
		}
	}
	if p[1856] != 0 {
		t.Fatal("output bias must start at zero")
	}
}

func TestOwnedParametersAndSeparateSGDUpdate(t *testing.T) {
	p := denseParameters()
	n := mustNetwork(t, p)
	want := slices.Clone(p)
	p[0] = 123
	exported := n.Parameters()
	exported[0] = 456
	if !slices.Equal(want, n.Parameters()) {
		t.Fatal("parameter slices alias the model")
	}
	input := denseInput()
	value, gradient, err := n.ValueGradient(input)
	if err != nil {
		t.Fatal(err)
	}
	const target, alpha = .5, .01
	updated := n.Parameters()
	for i := range updated {
		updated[i] += alpha * (target - value) * gradient[i]
	}
	next := mustNetwork(t, updated)
	nextValue, err := next.Forward(input)
	if err != nil || math.Abs(target-nextValue) >= math.Abs(target-value) || !slices.Equal(want, n.Parameters()) {
		t.Fatal("SGD update must reduce this fixture's error without mutating the old model")
	}
	// Shared inference has no mutable scratch buffers, including gradients.
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			v, g, err := n.ValueGradient(input)
			if err != nil || v != value || g != gradient {
				t.Error("concurrent evaluation changed the result")
			}
		})
	}
	workers.Wait()
}

func TestRejectInvalidParameters(t *testing.T) {
	for _, length := range []int{0, 1856, 1858} {
		if _, err := neural.New(make([]float64, length)); err == nil {
			t.Fatalf("accepted parameter count %d", length)
		}
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		for _, index := range []int{0, 1792, 1824, 1856} {
			p := make([]float64, 1857)
			p[index] = value
			if _, err := neural.New(p); err == nil {
				t.Fatalf("accepted nonfinite parameter %d", index)
			}
		}
	}
}

func TestRejectInvalidEvaluationWithoutPartialOutput(t *testing.T) {
	hiddenOverflow, outputOverflow, gradientOverflow := make([]float64, 1857), make([]float64, 1857), make([]float64, 1857)
	hiddenOverflow[0] = math.MaxFloat64
	outputOverflow[1792], outputOverflow[1824], outputOverflow[1856] = 1, math.MaxFloat64, math.MaxFloat64
	gradientOverflow[1824] = math.MaxFloat64
	for _, test := range []struct {
		name         string
		network      *neural.Network
		input        encoder.Vector
		gradientOnly bool
	}{
		{"nil network", nil, encoder.Vector{}, false},
		{"NaN input", &neural.Network{}, encoder.Vector{0: math.NaN()}, false},
		{"positive infinity input", &neural.Network{}, encoder.Vector{55: math.Inf(1)}, false},
		{"negative infinity input", &neural.Network{}, encoder.Vector{27: math.Inf(-1)}, false},
		{"hidden overflow", networkPointer(t, hiddenOverflow), encoder.Vector{0: 2}, false},
		{"output overflow", networkPointer(t, outputOverflow), encoder.Vector{}, false},
		{"gradient overflow", networkPointer(t, gradientOverflow), encoder.Vector{0: 2}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			if !test.gradientOnly {
				value, err := test.network.Forward(test.input)
				if err == nil || value != 0 {
					t.Fatalf("invalid forward must fail with zero output: %v, %v", value, err)
				}
			}
			value, gradient, err := test.network.ValueGradient(test.input)
			if err == nil || value != 0 || gradient != (neural.Gradient{}) {
				t.Fatalf("invalid gradient must fail with zero output: %v, %v", value, err)
			}
		})
	}
}

func denseParameters() []float64 {
	p := make([]float64, 1857)
	for i := range p {
		p[i] = .035 * math.Sin(float64(i+1)*.37)
	}
	return p
}

func denseInput() encoder.Vector {
	var input encoder.Vector
	for i := range input {
		input[i] = .1 + .8*float64(i+1)/56
	}
	return input
}

func mustNetwork(t *testing.T, p []float64) neural.Network {
	t.Helper()
	n, err := neural.New(p)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func networkPointer(t *testing.T, p []float64) *neural.Network {
	t.Helper()
	n := mustNetwork(t, p)
	return &n
}
