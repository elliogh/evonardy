// Package neural implements the shared float64 tanh value network.
package neural

import (
	"fmt"
	"math"

	"evonardy/internal/encoder"
	"evonardy/internal/random"
)

const Version = "tanh-56-32-1-v1"
const Inputs = encoder.Size
const Hidden = 32
const Outputs = 1
const ParameterCount = Hidden*Inputs + Hidden + Hidden + Outputs

const hiddenBiasOffset = Hidden * Inputs
const outputWeightOffset = hiddenBiasOffset + Hidden
const outputBiasIndex = outputWeightOffset + Hidden

// Network owns its parameters. Its zero value is a valid all-zero network.
// Evaluation is read-only and safe to share across goroutines.
type Network struct {
	parameters [ParameterCount]float64
}

// Gradient is dV/dtheta, ordered exactly like Parameters, not a loss gradient.
type Gradient [ParameterCount]float64

// New copies a flat parameter vector: W1 row-major, b1, W2, b2.
func New(parameters []float64) (Network, error) {
	if len(parameters) != ParameterCount {
		return Network{}, fmt.Errorf("network requires %d parameters, got %d", ParameterCount, len(parameters))
	}
	var n Network
	for i, value := range parameters {
		if !finite(value) {
			return Network{}, fmt.Errorf("nonfinite parameter %d", i)
		}
		n.parameters[i] = value
	}
	return n, nil
}

// Initialize uses an independent keyed PCG stream, Xavier-normal weights, and
// zero biases. The same seed and index reproduce the same network on a platform.
func Initialize(seed, index uint64) Network {
	r := random.New(seed, "neural/initialization/v1", index)
	var n Network
	for i := 0; i < hiddenBiasOffset; i++ {
		n.parameters[i] = r.NormFloat64() * math.Sqrt(2.0/(Inputs+Hidden))
	}
	for i := outputWeightOffset; i < outputBiasIndex; i++ {
		n.parameters[i] = r.NormFloat64() * math.Sqrt(2.0/(Hidden+Outputs))
	}
	return n
}

// Parameters returns a copy suitable for constructing a separately updated model.
func (n Network) Parameters() []float64 {
	parameters := make([]float64, ParameterCount)
	copy(parameters, n.parameters[:])
	return parameters
}

// Forward evaluates one finite input. Encoder semantics remain the caller's
// responsibility; the numerical network does not inspect game positions.
func (n *Network) Forward(input encoder.Vector) (float64, error) {
	value, _, err := n.forward(input)
	return value, err
}

// ValueGradient returns the value and its derivative at the same parameters.
// For TD/SGD, callers can use theta += alpha * (target-value) * gradient.
// No target, learning state, randomness, or update is part of evaluation.
func (n *Network) ValueGradient(input encoder.Vector) (float64, Gradient, error) {
	value, hidden, err := n.forward(input)
	if err != nil {
		return 0, Gradient{}, err
	}
	var gradient Gradient
	outputDerivative := 1 - value*value
	gradient[outputBiasIndex] = outputDerivative
	for j, activation := range hidden {
		gradient[outputWeightOffset+j] = outputDerivative * activation
		hiddenDerivative := outputDerivative * n.parameters[outputWeightOffset+j] * (1 - activation*activation)
		gradient[hiddenBiasOffset+j] = hiddenDerivative
		for i, x := range input {
			gradient[j*Inputs+i] = hiddenDerivative * x
		}
	}
	for i, derivative := range gradient {
		if !finite(derivative) {
			return 0, Gradient{}, fmt.Errorf("nonfinite gradient %d", i)
		}
	}
	return value, gradient, nil
}

func (n *Network) forward(input encoder.Vector) (float64, [Hidden]float64, error) {
	var hidden [Hidden]float64
	if n == nil {
		return 0, hidden, fmt.Errorf("nil network")
	}
	for i, x := range input {
		if !finite(x) {
			return 0, hidden, fmt.Errorf("nonfinite input %d", i)
		}
	}
	for j := range hidden {
		z := n.parameters[hiddenBiasOffset+j]
		for i, x := range input {
			z += n.parameters[j*Inputs+i] * x
		}
		if !finite(z) {
			return 0, [Hidden]float64{}, fmt.Errorf("nonfinite hidden preactivation %d", j)
		}
		hidden[j] = math.Tanh(z)
	}
	z := n.parameters[outputBiasIndex]
	for j, activation := range hidden {
		z += n.parameters[outputWeightOffset+j] * activation
	}
	if !finite(z) {
		return 0, [Hidden]float64{}, fmt.Errorf("nonfinite output preactivation")
	}
	return math.Tanh(z), hidden, nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
