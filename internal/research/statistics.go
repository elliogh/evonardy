// Package research compares frozen policies without treating paired games as independent.
package research

import (
	"fmt"
	"math"
	"slices"

	"evonardy/internal/game"
	"evonardy/internal/random"
	"evonardy/internal/replay"
	"evonardy/internal/training"
)

const StatisticsVersion = "seed-stratified-pair-percentile-v1"

type Bootstrap struct {
	Seed       uint64  `json:"seed"`
	Resamples  int     `json:"resamples"`
	Confidence float64 `json:"confidence"`
}

func DefaultBootstrap() Bootstrap { return Bootstrap{Seed: 2026, Resamples: 2000, Confidence: .95} }
func (b Bootstrap) Validate() error {
	if b.Resamples < 100 || b.Resamples > 10000 || math.IsNaN(b.Confidence) || b.Confidence < .8 || b.Confidence > .99 {
		return fmt.Errorf("bootstrap requires 100..10000 resamples and confidence in [0.8,0.99]")
	}
	return nil
}

type Interval struct {
	Low  float64 `json:"low"`
	High float64 `json:"high"`
}
type Estimate struct {
	Version       string    `json:"version"`
	Bootstrap     Bootstrap `json:"bootstrap"`
	TrainingSeeds int       `json:"training_seeds"`
	Pairs         int       `json:"pairs"`
	Games         int       `json:"games"`
	WinRate       float64   `json:"win_rate"`
	MeanPoints    float64   `json:"mean_points"`
	MarsWinRate   float64   `json:"mars_win_rate"`
	WinInterval   Interval  `json:"win_interval"`
	PointInterval Interval  `json:"point_interval"`
	Degenerate    bool      `json:"degenerate"`
}
type pairValue struct {
	win, points, mars float64
	seed              uint64
	opponent          string
}

func pairs(scores []training.Score) ([]pairValue, error) {
	if len(scores) == 0 || len(scores)%2 != 0 || len(scores) > 1000 {
		return nil, fmt.Errorf("expected 1..500 complete pairs")
	}
	out := make([]pairValue, 0, len(scores)/2)
	seen := map[string]bool{}
	for i := 0; i < len(scores); i += 2 {
		a, b := scores[i], scores[i+1]
		if a.Index != i || b.Index != i+1 || a.Side != game.White || b.Side != game.Black || a.Seed != b.Seed || a.OpponentID == "" || a.OpponentID != b.OpponentID {
			return nil, fmt.Errorf("pair schedule mismatch at %d", i)
		}
		key := fmt.Sprintf("%s/%d", a.OpponentID, a.Seed)
		if seen[key] {
			return nil, fmt.Errorf("duplicate independent pair")
		}
		seen[key] = true
		p := pairValue{seed: a.Seed, opponent: a.OpponentID}
		for _, s := range []training.Score{a, b} {
			if err := training.ValidScore(s, replay.MaxEvents); err != nil {
				return nil, err
			}
			if s.Status != replay.Completed {
				return nil, fmt.Errorf("truncated evaluation has no estimate")
			}
			sign := -1.
			if s.Outcome.Winner == s.Side {
				sign = 1
				p.win += .5
				if s.Outcome.Mars {
					p.mars += .5
				}
			}
			p.points += sign * float64(s.Outcome.Points) / 2
		}
		out = append(out, p)
	}
	return out, nil
}
func quantile(sorted []float64, q float64) float64 {
	x := q * float64(len(sorted)-1)
	i := int(x)
	return sorted[i] + (sorted[min(i+1, len(sorted)-1)]-sorted[i])*(x-float64(i))
}
func interval(samples []float64, b Bootstrap) Interval {
	slices.Sort(samples)
	tail := (1 - b.Confidence) / 2
	return Interval{quantile(samples, tail), quantile(samples, 1-tail)}
}

// Analyze resamples training seed clusters, then independent dice pairs within
// each opponent stratum. Opponents receive equal weight. Correlated sides stay together.
func Analyze(runs [][]training.Score, b Bootstrap) (Estimate, error) {
	return analyze(runs, nil, b)
}

// Compare preserves the same training-seed and pair resampling indices for both
// methods. WinRate and intervals describe A minus B, not a head-to-head win rate.
func Compare(a, b [][]training.Score, bootstrap Bootstrap) (Estimate, error) {
	return analyze(a, b, bootstrap)
}
func analyze(runs, other [][]training.Score, b Bootstrap) (Estimate, error) {
	if err := b.Validate(); err != nil {
		return Estimate{}, err
	}
	if len(runs) < 1 || len(runs) > 20 || (other != nil && len(other) != len(runs)) {
		return Estimate{}, fmt.Errorf("expected matching 1..20 seed cohorts")
	}
	cohorts := make([]map[string][]pairValue, len(runs))
	names := []string{}
	total := 0
	for n, scores := range runs {
		ps, err := pairs(scores)
		if err != nil {
			return Estimate{}, err
		}
		total += len(ps)
		if other != nil {
			bs, err := pairs(other[n])
			if err != nil {
				return Estimate{}, err
			}
			if len(ps) != len(bs) {
				return Estimate{}, fmt.Errorf("comparison pair count mismatch")
			}
			for j := range ps {
				if ps[j].seed != bs[j].seed || ps[j].opponent != bs[j].opponent {
					return Estimate{}, fmt.Errorf("comparison schedule mismatch")
				}
				ps[j].win -= bs[j].win
				ps[j].points -= bs[j].points
				ps[j].mars -= bs[j].mars
			}
		}
		cohorts[n] = map[string][]pairValue{}
		for _, p := range ps {
			cohorts[n][p.opponent] = append(cohorts[n][p.opponent], p)
		}
		current := []string{}
		for name := range cohorts[n] {
			current = append(current, name)
		}
		slices.Sort(current)
		if n == 0 {
			names = current
		} else if !slices.Equal(names, current) {
			return Estimate{}, fmt.Errorf("opponent strata differ across seeds")
		}
		if n > 0 {
			for _, name := range names {
				if len(cohorts[n][name]) != len(cohorts[0][name]) {
					return Estimate{}, fmt.Errorf("pair quotas differ across seeds")
				}
			}
		}
	}
	if int64(total)*int64(b.Resamples) > 20000000 {
		return Estimate{}, fmt.Errorf("bootstrap exceeds 20000000 pair draws")
	}
	e := Estimate{Version: StatisticsVersion, Bootstrap: b, TrainingSeeds: len(runs), Pairs: total, Games: total * 2}
	weight := 1 / float64(len(runs)*len(names))
	for _, c := range cohorts {
		for _, name := range names {
			ps := c[name]
			var win, point, mars float64
			for _, p := range ps {
				win += p.win
				point += p.points
				mars += p.mars
			}
			e.WinRate += win / float64(len(ps)) * weight
			e.MeanPoints += point / float64(len(ps)) * weight
			e.MarsWinRate += mars / float64(len(ps)) * weight
		}
	}
	wins, points := make([]float64, b.Resamples), make([]float64, b.Resamples)
	rng := random.New(b.Seed, StatisticsVersion, 0)
	for r := range wins {
		for range cohorts {
			c := cohorts[rng.IntN(len(cohorts))]
			for _, name := range names {
				ps := c[name]
				var win, point float64
				for range ps {
					p := ps[rng.IntN(len(ps))]
					win += p.win
					point += p.points
				}
				wins[r] += win / float64(len(ps)) * weight
				points[r] += point / float64(len(ps)) * weight
			}
		}
	}
	e.WinInterval = interval(wins, b)
	e.PointInterval = interval(points, b)
	e.Degenerate = e.WinInterval.Low == e.WinInterval.High
	return e, nil
}

type ConfirmationProtocol struct {
	Version       string    `json:"version"`
	Pairs         int       `json:"pairs"`
	MinimumPairs  int       `json:"minimum_pairs"`
	MinimumMargin float64   `json:"minimum_margin"`
	Bootstrap     Bootstrap `json:"bootstrap"`
}

const ConfirmationVersion = "single-heldout-incumbent-v1"

func DefaultConfirmation() ConfirmationProtocol {
	return ConfirmationProtocol{Version: ConfirmationVersion, Pairs: 100, MinimumPairs: 100, MinimumMargin: 0, Bootstrap: DefaultBootstrap()}
}
func (p ConfirmationProtocol) Validate() error {
	if p.Version != ConfirmationVersion || p.Pairs < 1 || p.Pairs > 500 || p.MinimumPairs < 30 || p.MinimumPairs > 500 || math.IsNaN(p.MinimumMargin) || p.MinimumMargin < 0 || p.MinimumMargin >= .5 {
		return fmt.Errorf("invalid predeclared confirmation: 1..500 pairs, minimum 30..500 pairs, margin [0,0.5)")
	}
	return p.Bootstrap.Validate()
}

type Verdict struct {
	Status   string   `json:"status"`
	Reason   string   `json:"reason"`
	Estimate Estimate `json:"estimate"`
}

// Confirm is called once for a candidate selected entirely on development data.
// It never modifies a model or global champion pointer.
func Confirm(scores []training.Score, p ConfirmationProtocol) (Verdict, error) {
	if err := p.Validate(); err != nil {
		return Verdict{}, err
	}
	ps, err := pairs(scores)
	if err != nil {
		return Verdict{}, err
	}
	if len(ps) != p.Pairs {
		return Verdict{}, fmt.Errorf("final pair quota differs from predeclared protocol")
	}
	opponent := ps[0].opponent
	for _, v := range ps {
		if v.opponent != opponent {
			return Verdict{}, fmt.Errorf("confirmation requires one frozen incumbent")
		}
	}
	e, err := Analyze([][]training.Score{scores}, p.Bootstrap)
	if err != nil {
		return Verdict{}, err
	}
	v := Verdict{Status: "candidate", Reason: "Insufficient independent pairs", Estimate: e}
	if len(ps) >= p.MinimumPairs {
		v.Reason = "Confidence interval does not establish the predeclared margin"
		if e.Degenerate {
			v.Reason = "Degenerate bootstrap cannot establish confirmation"
		} else if e.WinInterval.Low > .5+p.MinimumMargin {
			v.Status = "confirmed"
			v.Reason = "Held-out lower confidence bound exceeds the predeclared margin"
		}
	}
	return v, nil
}
