package agent

import (
	"evonardy/internal/features"
	"fmt"
	"math"
)

// Policy is a frozen, serializable inference description. Each game owns its RNG.
type Policy struct {
	ID      string          `json:"id"`
	Kind    string          `json:"kind"`
	Weights features.Vector `json:"weights"`
}

func (p Policy) Validate() error {
	if p.ID == "" || (p.Kind != "linear" && p.Kind != "random") {
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
	return nil
}
func (p Policy) New(source IntSource) Agent {
	if p.Kind == "random" {
		return NewRandom(source)
	}
	return Linear{Weights: p.Weights}
}
