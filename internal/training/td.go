package training

import (
	"fmt"
	"math"

	"evonardy/internal/encoder"
	"evonardy/internal/game"
	"evonardy/internal/neural"
)

const TDAlgorithm = "td-zero-v1"
const TDLambdaAlgorithm = "td-lambda-v1"
const TDRandomContract = "keyed-pcg-td-v1"

// TDLearner owns one sequential learner. Inference snapshots own separate copies.
type TDLearner struct {
	network neural.Network
	alpha   float64
	lambda  float64
	traces  neural.Gradient
}

type TDUpdate struct {
	Value              float64
	Target             float64
	Delta              float64
	Terminal           bool
	ForwardEvaluations int
}

func NewTDLearner(parameters []float64, alpha float64) (*TDLearner, error) {
	return NewTDLambda(parameters, alpha, 0)
}

// NewTDLambda starts with empty accumulating traces, including replacement models.
func NewTDLambda(parameters []float64, alpha, lambda float64) (*TDLearner, error) {
	if !finiteTD(alpha) || alpha <= 0 || alpha > 1 {
		return nil, fmt.Errorf("alpha must be finite in (0,1]")
	}
	if !finiteTD(lambda) || lambda < 0 || lambda > 1 {
		return nil, fmt.Errorf("lambda must be finite in [0,1]")
	}
	n, err := neural.New(parameters)
	if err != nil {
		return nil, err
	}
	for _, parameter := range parameters {
		if math.Abs(parameter) > 1e6 {
			return nil, fmt.Errorf("TD parameters exceed inference bounds")
		}
	}
	return &TDLearner{network: n, alpha: alpha, lambda: lambda}, nil
}

func (l *TDLearner) Parameters() []float64 { return l.network.Parameters() }

func (l *TDLearner) Traces() neural.Gradient { return l.traces }
func (l *TDLearner) ResetTraces()            { l.traces = neural.Gradient{} }

// Update treats the bootstrap target as constant and never flips the White value
// perspective. The caller supplies completed Go transitions, not draft previews.
func (l *TDLearner) Update(current, next game.Position) (TDUpdate, error) {
	var report TDUpdate
	input, err := encoder.Encode(current)
	if err != nil {
		return report, err
	}
	if _, terminal := game.Result(current); terminal {
		return report, fmt.Errorf("cannot update a terminal current position")
	}
	if err := game.ValidatePosition(next); err != nil {
		return report, err
	}
	value, gradient, err := l.network.ValueGradient(input)
	if err != nil {
		return report, err
	}
	report.Value, report.ForwardEvaluations = value, 1
	if result, terminal := game.Result(next); terminal {
		report.Terminal = true
		report.Target = -1
		if result.Winner == game.White {
			report.Target = 1
		}
	} else {
		nextInput, err := encoder.Encode(next)
		if err != nil {
			return TDUpdate{}, err
		}
		report.Target, err = l.network.Forward(nextInput)
		if err != nil {
			return TDUpdate{}, err
		}
		report.ForwardEvaluations++
	}
	report.Delta = report.Target - value
	parameters := l.network.Parameters()
	var traces neural.Gradient
	for i, derivative := range gradient {
		traces[i] = l.lambda*l.traces[i] + derivative
		if !finiteTD(traces[i]) {
			return TDUpdate{}, fmt.Errorf("nonfinite TD trace %d", i)
		}
		parameters[i] += l.alpha * report.Delta * traces[i]
		if !finiteTD(parameters[i]) || math.Abs(parameters[i]) > 1e6 {
			return TDUpdate{}, fmt.Errorf("TD update produced invalid parameter %d", i)
		}
	}
	updated, err := neural.New(parameters)
	if err != nil {
		return TDUpdate{}, err
	}
	l.network = updated
	l.traces = traces
	if report.Terminal {
		l.ResetTraces()
	}
	return report, nil
}

func finiteTD(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
