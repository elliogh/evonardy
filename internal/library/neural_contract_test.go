package library

import (
	"math"
	"testing"

	"evonardy/internal/encoder"
	"evonardy/internal/neural"
)

func TestNeuralShapeAndVersionValidation(t *testing.T) {
	manifest := Manifest{Evaluator: neural.Version, FeaturesVersion: encoder.Version, Architecture: []int{56, 32, 1}, Inference: Inference{TieBreak: "stable_first"}}
	model := Model{Weights: neural.Initialize(42, 0).Parameters()}
	if err := validateModel(manifest, model); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Manifest, *Model){
		func(m *Manifest, _ *Model) { m.Evaluator = "tanh-56-32-1-v2" },
		func(m *Manifest, _ *Model) { m.FeaturesVersion = "unsupported" },
		func(m *Manifest, _ *Model) { m.Architecture = []int{56, 31, 1} },
		func(m *Manifest, _ *Model) { m.Architecture = []int{56, 32, 2} },
		func(m *Manifest, _ *Model) { m.Inference.TieBreak = "random" },
		func(_ *Manifest, m *Model) { m.Weights = m.Weights[:1856] },
		func(_ *Manifest, m *Model) { m.Weights = append(m.Weights, 1) },
		func(_ *Manifest, m *Model) { m.Weights[0] = math.NaN() },
		func(_ *Manifest, m *Model) { m.Weights[1856] = math.Inf(1) },
		func(_ *Manifest, m *Model) { m.Weights[1792] = 1e7 },
	} {
		m, weights := manifest, Model{Weights: append([]float64{}, model.Weights...)}
		mutate(&m, &weights)
		if err := validateModel(m, weights); err == nil {
			t.Fatal("accepted incompatible neural package")
		}
	}
}
