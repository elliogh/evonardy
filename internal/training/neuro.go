package training

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sync"

	"evonardy/internal/agent"
	"evonardy/internal/arena"
	"evonardy/internal/encoder"
	"evonardy/internal/features"
	"evonardy/internal/game"
	"evonardy/internal/neural"
	"evonardy/internal/random"
)

const GAMLPAlgorithm = "ga-mlp-v1"
const NeuroRandomContract = "keyed-pcg-neuro-v1"

type NeuroCandidate struct {
	ID         string                 `json:"id"`
	Parameters agent.NeuralParameters `json:"parameters"`
	Parents    []string               `json:"parents"`
	Reason     string                 `json:"reason"`
	Stats      *Stats                 `json:"stats"`
}
type NeuroGeneration struct {
	Algorithm      string           `json:"algorithm"`
	Ruleset        string           `json:"ruleset"`
	EncoderVersion string           `json:"encoder_version"`
	NetworkVersion string           `json:"network_version"`
	Number         int              `json:"number"`
	Ranked         []NeuroCandidate `json:"ranked"`
	Scores         []Score          `json:"scores"`
	Metric         Metric           `json:"metric"`
}
type NeuroState struct {
	Version        int              `json:"version"`
	Algorithm      string           `json:"algorithm"`
	RandomContract string           `json:"random_contract"`
	Ruleset        string           `json:"ruleset"`
	EncoderVersion string           `json:"encoder_version"`
	NetworkVersion string           `json:"network_version"`
	Config         Config           `json:"config"`
	Development    [2]agent.Policy  `json:"development"`
	Generation     int              `json:"generation"`
	Population     []NeuroCandidate `json:"population"`
	Results        []Score          `json:"results"`
	History        []Metric         `json:"history"`
	Counters       Counters         `json:"counters"`
}

func DefaultGAMLPConfig() Config {
	c := DefaultConfig()
	c.Population = 8
	c.Generations = 4
	c.InitialSigma = 1
	c.MutationSigma = .02
	return c
}
func NewGAMLP(c Config) (NeuroState, error) {
	if err := c.Validate(); err != nil {
		return NeuroState{}, err
	}
	s := NeuroState{Version: 1, Algorithm: GAMLPAlgorithm, RandomContract: NeuroRandomContract, Ruleset: game.Ruleset, EncoderVersion: encoder.Version, NetworkVersion: neural.Version, Config: c, Development: [2]agent.Policy{{ID: "builtin/heuristic-v1", Kind: "linear", Weights: features.DefaultWeights}, {ID: "builtin/random-v1", Kind: "random"}}, Results: []Score{}, History: []Metric{}}
	for i := 0; i < c.Population; i++ {
		p := NeuroCandidate{ID: neuroID(0, i), Parents: []string{}, Reason: "initialization"}
		copy(p.Parameters[:], neural.Initialize(c.Seed, uint64(i)).Parameters())
		for j := range p.Parameters {
			p.Parameters[j] *= c.InitialSigma
		}
		s.Population = append(s.Population, p)
	}
	return s, nil
}
func neuroID(g, i int) string { return fmt.Sprintf("n%04d-c%04d", g, i) }
func (s NeuroState) schedule() State {
	return State{Config: s.Config, Generation: s.Generation, Results: s.Results}
}
func NeuroSlots(s NeuroState) int { return Slots(s.schedule()) }
func PlayNeuroWave(ctx context.Context, s NeuroState) (Wave, error) {
	tasks := NextTasks(s.schedule())
	if len(tasks) == 0 {
		return Wave{}, fmt.Errorf("neural selection budget exhausted")
	}
	scores := make([]Score, len(tasks))
	errs := make([]error, len(tasks))
	var sample *PlayedGame
	var wg sync.WaitGroup
	for i, t := range tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := s.Population[t.Candidate]
			candidate := agent.Policy{ID: p.ID, Kind: "neural", NeuralParameters: p.Parameters, EncoderVersion: encoder.Version, NetworkVersion: neural.Version}
			opponent := s.Development[t.Opponent]
			factories := [2]arena.Factory{}
			factories[t.Side] = arena.Factory{ID: p.ID, New: candidate.New}
			factories[t.Side.Other()] = arena.Factory{ID: opponent.ID, New: opponent.New}
			r, err := arena.Play(ctx, arena.MatchConfig{Seed: t.Seed, ID: uint64(t.Index), StreamID: 0, MaxTurns: s.Config.MaxTurns}, factories)
			errs[i] = err
			if err == nil {
				scores[i] = Score{Index: t.Index, Seed: t.Seed, Side: t.Side, OpponentID: opponent.ID, Status: r.Record.Status, Outcome: r.Record.Outcome, Turns: r.Decisions, ForwardEvaluations: r.ForwardEvaluations}
				if i == 0 {
					sample = &PlayedGame{Generation: s.Generation + 1, Index: t.Index, CandidateID: p.ID, OpponentID: opponent.ID, CandidateSide: t.Side, Replay: r.Record}
				}
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return Wave{}, err
		}
	}
	return Wave{Scores: scores, Sample: sample}, nil
}
func AddNeuro(s NeuroState, scores []Score) (NeuroState, error) {
	proxy := s.schedule()
	proxy.Development = s.Development
	proxy.Counters = s.Counters
	next, err := Add(proxy, scores)
	s.Results, s.Counters = next.Results, next.Counters
	return s, err
}
func NeuroReady(s NeuroState) bool { return Ready(s.schedule()) }

// RankNeuro evaluates frozen copies. No update or random draw occurs here.
func RankNeuro(s NeuroState) (NeuroGeneration, error) {
	if !NeuroReady(s) {
		return NeuroGeneration{}, fmt.Errorf("neural selection incomplete")
	}
	ranked := append([]NeuroCandidate{}, s.Population...)
	m := Metric{Generation: s.Generation + 1, Games: len(s.Results)}
	for i := range ranked {
		stats, err := Summarize(s.Results[i*s.Config.PairsPerOpponent*4 : (i+1)*s.Config.PairsPerOpponent*4])
		if err != nil {
			return NeuroGeneration{}, err
		}
		ranked[i].Parents = slices.Clone(ranked[i].Parents)
		ranked[i].Stats = &stats
		m.MeanFitness += stats.Fitness / float64(len(ranked))
	}
	for _, r := range s.Results {
		m.Decisions += uint64(r.Turns)
		m.ForwardEvaluations += uint64(r.ForwardEvaluations)
	}
	slices.SortStableFunc(ranked, func(a, b NeuroCandidate) int {
		if a.Stats.Fitness > b.Stats.Fitness {
			return -1
		}
		if a.Stats.Fitness < b.Stats.Fitness {
			return 1
		}
		return 0
	})
	m.BestFitness = ranked[0].Stats.Fitness
	m.BestCandidateID = ranked[0].ID
	scores := slices.Clone(s.Results)
	for i := range scores {
		if scores[i].Outcome != nil {
			outcome := *scores[i].Outcome
			scores[i].Outcome = &outcome
		}
	}
	return NeuroGeneration{Algorithm: s.Algorithm, Ruleset: s.Ruleset, EncoderVersion: s.EncoderVersion, NetworkVersion: s.NetworkVersion, Number: s.Generation + 1, Ranked: ranked, Scores: scores, Metric: m}, nil
}
func AdvanceNeuro(s NeuroState) (NeuroState, NeuroGeneration, error) {
	gen, err := RankNeuro(s)
	if err != nil {
		return s, gen, err
	}
	s.Generation++
	s.History = append(slices.Clone(s.History), gen.Metric)
	s.Results = []Score{}
	if s.Generation == s.Config.Generations {
		s.Population = gen.Ranked
		return s, gen, nil
	}
	elite := max(1, int(math.Ceil(float64(s.Config.Population)*s.Config.EliteFraction)))
	selection := random.New(s.Config.Seed, "neuro/selection", uint64(s.Generation))
	population := make([]NeuroCandidate, s.Config.Population)
	for i := range population {
		parent := i
		reason := "elite"
		if i >= elite {
			reason = "mutation"
			parent = selection.IntN(len(gen.Ranked))
			for j := 1; j < s.Config.TournamentSize; j++ {
				parent = min(parent, selection.IntN(len(gen.Ranked)))
			}
		}
		a := gen.Ranked[parent]
		child := NeuroCandidate{ID: neuroID(s.Generation, i), Parameters: a.Parameters, Parents: []string{a.ID}, Reason: reason}
		if i >= elite {
			r := random.New(s.Config.Seed, fmt.Sprintf("neuro/mutation/%d", s.Generation), uint64(i))
			for j, w := range child.Parameters {
				child.Parameters[j] = max(-1e6, min(1e6, w+r.NormFloat64()*s.Config.MutationSigma))
				if child.Parameters[j] != w {
					s.Counters.Mutations++
				}
			}
		}
		population[i] = child
	}
	s.Population = population
	return s, gen, nil
}
func ValidateNeuro(s NeuroState) error {
	if err := s.Config.Validate(); err != nil {
		return err
	}
	if s.Version != 1 || s.Algorithm != GAMLPAlgorithm || s.RandomContract != NeuroRandomContract || s.Ruleset != game.Ruleset || s.EncoderVersion != encoder.Version || s.NetworkVersion != neural.Version {
		return fmt.Errorf("incompatible neural population")
	}
	proxy := s.schedule()
	proxy.Version = 1
	proxy.Algorithm = Algorithm
	proxy.RandomContract = RandomContract
	proxy.Ruleset = game.Ruleset
	proxy.FeaturesVersion = features.Version
	proxy.Development = s.Development
	proxy.History = s.History
	proxy.Counters = s.Counters
	for _, p := range s.Population {
		if _, err := NewTDLearner(p.Parameters[:], .001); err != nil {
			return err
		}
		proxy.Population = append(proxy.Population, Candidate{ID: p.ID})
	}
	if s.Counters.Crossovers != 0 {
		return fmt.Errorf("GA-MLP does not use crossover")
	}
	elite := max(1, int(math.Ceil(float64(s.Config.Population)*s.Config.EliteFraction)))
	expected := uint64(min(s.Generation, s.Config.Generations-1) * (s.Config.Population - elite) * neural.ParameterCount)
	// A bounded mutation may leave a parameter unchanged at a numerical limit.
	if s.Counters.Mutations > expected {
		return fmt.Errorf("invalid neural mutation count")
	}
	return Validate(proxy)
}
