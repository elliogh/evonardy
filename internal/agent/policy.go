package agent

import (
	"evonardy/internal/encoder"
	"evonardy/internal/features"
	"evonardy/internal/neural"
	"fmt"
	"math"
)

// Policy is a frozen, serializable inference description. Each game owns its RNG.
type Policy struct {
	ID               string           `json:"id"`
	Kind             string           `json:"kind"`
	Weights          features.Vector  `json:"weights"`
	NeuralParameters NeuralParameters `json:"neural_parameters,omitzero"`
	NetworkVersion   string           `json:"network_version,omitempty"`
	EncoderVersion   string           `json:"encoder_version,omitempty"`
}

func (p Policy) Validate() error {
	if p.ID == "" || (p.Kind != "linear" && p.Kind != "random" && p.Kind != "neural") {
		return fmt.Errorf("invalid frozen policy")
	}
	for _, w := range p.Weights {
		if math.IsNaN(w) || math.IsInf(w, 0) || math.Abs(w) > 1e6 {
			return fmt.Errorf("invalid policy weights")
		}
	}
	if p.Kind == "random" && p.Weights != (features.Vector{}) {
		return fmt.Errorf("random policy has weights")
	}
	if p.Kind != "neural" {
		if p.NeuralParameters != (NeuralParameters{}) || p.NetworkVersion != "" || p.EncoderVersion != "" {
			return fmt.Errorf("non-neural policy has neural data")
		}
		return nil
	}
	if p.Weights != (features.Vector{}) || p.NetworkVersion != neural.Version || p.EncoderVersion != encoder.Version {
		return fmt.Errorf("incompatible neural policy")
	}
	if _, err := neural.New(p.NeuralParameters[:]); err != nil {
		return err
	}
	for _, w := range p.NeuralParameters {
		if math.Abs(w) > 1e6 {
			return fmt.Errorf("invalid neural policy weights")
		}
	}
	return nil
}
func (p Policy) New(source IntSource) Agent {
	if p.Kind == "neural" {
		if err := p.Validate(); err != nil {
			return nil
		}
		n, err := NewNeural(p.NeuralParameters[:])
		if err != nil {
			return nil
		}
		return n
	}
	if p.Kind == "random" {
		return NewRandom(source)
	}
	return Linear{Weights: p.Weights}
}
