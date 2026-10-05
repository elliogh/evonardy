// Package training implements versioned GA-linear evolution at durable game boundaries.
package training

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"

	"evonardy/internal/agent"
	"evonardy/internal/arena"
	"evonardy/internal/features"
	"evonardy/internal/game"
	"evonardy/internal/random"
	"evonardy/internal/replay"
)

const Algorithm = "ga-linear-v1"
const RandomContract = "keyed-pcg-ga-v1"

var ErrTruncated = errors.New("selection game truncated; no generation fitness or selection")

type Config struct {
	Seed             uint64  `json:"seed"`
	Population       int     `json:"population"`
	Generations      int     `json:"generations"`
	PairsPerOpponent int     `json:"pairs_per_opponent"`
	Workers          int     `json:"workers"`
	MaxTurns         int     `json:"max_turns"`
	EliteFraction    float64 `json:"elite_fraction"`
	TournamentSize   int     `json:"tournament_size"`
	InitialSigma     float64 `json:"initial_sigma"`
	MutationSigma    float64 `json:"mutation_sigma"`
}

func DefaultConfig() Config {
	return Config{Seed: 42, Population: 64, Generations: 20, PairsPerOpponent: 2, Workers: 2, MaxTurns: 1200, EliteFraction: .1, TournamentSize: 3, InitialSigma: .4, MutationSigma: .15}
}
func (c Config) Validate() error {
	if c.Population < 4 || c.Population > 128 || c.Generations < 1 || c.Generations > 500 || c.PairsPerOpponent < 1 || c.PairsPerOpponent > 32 || c.Workers < 1 || c.Workers > 8 || c.MaxTurns < 1 || c.MaxTurns > replay.MaxEvents {
		return fmt.Errorf("population 4..128, generations 1..500, pairs 1..32, workers 1..8, max_turns 1..10000 required")
	}
	if c.TournamentSize < 2 || c.TournamentSize > c.Population {
		return fmt.Errorf("invalid tournament size")
	}
	for _, x := range []float64{c.EliteFraction, c.InitialSigma, c.MutationSigma} {
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return fmt.Errorf("nonfinite hyperparameter")
		}
	}
	if c.EliteFraction < .05 || c.EliteFraction > .5 || c.InitialSigma <= 0 || c.InitialSigma > 3 || c.MutationSigma <= 0 || c.MutationSigma > 3 {
		return fmt.Errorf("invalid elitism or mutation scale")
	}
	games := int64(c.Population) * int64(c.Generations) * int64(c.PairsPerOpponent) * 4
	if games > 200000 || games*int64(c.MaxTurns) > 200000000 {
		return fmt.Errorf("run exceeds 200000 games or 200000000 turn slots")
	}
	return nil
}

type Counters struct {
	Games              uint64 `json:"games"`
	CompletedGames     uint64 `json:"completed_games"`
	TruncatedGames     uint64 `json:"truncated_games"`
	Decisions          uint64 `json:"decisions"`
	ForwardEvaluations uint64 `json:"forward_evaluations"`
	Mutations          uint64 `json:"mutations"`
	Crossovers         uint64 `json:"crossovers"`
}
type Stats struct {
	Games    int     `json:"games"`
	Wins     int     `json:"wins"`
	MarsWins int     `json:"mars_wins"`
	Points   int     `json:"points"`
	Fitness  float64 `json:"fitness"`
}
type Candidate struct {
	ID      string          `json:"id"`
	Weights features.Vector `json:"weights"`
	Parents []string        `json:"parents"`
	Stats   *Stats          `json:"stats"`
}
type Score struct {
	Index              int           `json:"index"`
	Seed               uint64        `json:"seed"`
	Side               game.Player   `json:"side"`
	OpponentID         string        `json:"opponent_id"`
	Status             string        `json:"status"`
	Outcome            *game.Outcome `json:"outcome"`
	Turns              int           `json:"turns"`
	ForwardEvaluations int           `json:"forward_evaluations"`
}
type Metric struct {
	Generation         int     `json:"generation"`
	Games              int     `json:"games"`
	BestFitness        float64 `json:"best_fitness"`
	MeanFitness        float64 `json:"mean_fitness"`
	BestCandidateID    string  `json:"best_candidate_id"`
	Decisions          uint64  `json:"decisions"`
	ForwardEvaluations uint64  `json:"forward_evaluations"`
}
type Generation struct {
	Algorithm       string      `json:"algorithm"`
	Ruleset         string      `json:"ruleset"`
	FeaturesVersion string      `json:"features_version"`
	Number          int         `json:"number"`
	Ranked          []Candidate `json:"ranked"`
	Scores          []Score     `json:"scores"`
	Metric          Metric      `json:"metric"`
}
type State struct {
	Version         int             `json:"version"`
	Algorithm       string          `json:"algorithm"`
	RandomContract  string          `json:"random_contract"`
	Ruleset         string          `json:"ruleset"`
	FeaturesVersion string          `json:"features_version"`
	Config          Config          `json:"config"`
	Development     [2]agent.Policy `json:"development"`
	Generation      int             `json:"generation"`
	Population      []Candidate     `json:"population"`
	Results         []Score         `json:"results"`
	History         []Metric        `json:"history"`
	Counters        Counters        `json:"counters"`
}

func New(c Config) (State, error) {
	if err := c.Validate(); err != nil {
		return State{}, err
	}
	s := State{Version: 1, Algorithm: Algorithm, RandomContract: RandomContract, Ruleset: game.Ruleset, FeaturesVersion: features.Version, Config: c, Development: [2]agent.Policy{{ID: "builtin/heuristic-v1", Kind: "linear", Weights: features.DefaultWeights}, {ID: "builtin/random-v1", Kind: "random"}}, Population: []Candidate{}, Results: []Score{}, History: []Metric{}}
	for i := 0; i < c.Population; i++ {
		weights := features.DefaultWeights
		if i > 0 {
			r := random.New(c.Seed, "ga/initialization", uint64(i))
			for j := range weights {
				weights[j] = bound(weights[j] + r.NormFloat64()*c.InitialSigma)
			}
		}
		s.Population = append(s.Population, Candidate{ID: candidateID(0, i), Weights: weights, Parents: []string{}})
	}
	return s, nil
}
func candidateID(g, i int) string { return fmt.Sprintf("g%04d-c%04d", g, i) }
func bound(x float64) float64     { return max(-10, min(10, x)) }

type Task struct {
	Index, Candidate, Opponent, Pair int
	Side                             game.Player
	Seed                             uint64
}

func task(s State, index int) Task {
	perCandidate := s.Config.PairsPerOpponent * 4
	candidate := index / perCandidate
	within := index % perCandidate
	opponent := within / (s.Config.PairsPerOpponent * 2)
	pair := (within / 2) % s.Config.PairsPerOpponent
	return Task{Index: index, Candidate: candidate, Opponent: opponent, Pair: pair, Side: game.Player(index % 2), Seed: PairSeed(s.Config.Seed, "ga/development", opponent, pair)}
}
func PairSeed(seed uint64, domain string, opponent, pair int) uint64 {
	return random.New(seed, domain+fmt.Sprintf("/opponent/%d", opponent), uint64(pair)).Uint64()
}
func Slots(s State) int { return s.Config.Population * s.Config.PairsPerOpponent * 4 }
func NextTasks(s State) []Task {
	if s.Generation >= s.Config.Generations {
		return nil
	}
	n := Slots(s)
	tasks := []Task{}
	for i := len(s.Results); i < min(n, len(s.Results)+s.Config.Workers); i++ {
		tasks = append(tasks, task(s, i))
	}
	return tasks
}
func Play(ctx context.Context, s State, tasks []Task) ([]Score, error) {
	if len(tasks) == 0 || len(tasks) > s.Config.Workers {
		return nil, fmt.Errorf("invalid wave size")
	}
	scores := make([]Score, len(tasks))
	errs := make([]error, len(tasks))
	var wg sync.WaitGroup
	for _, t := range tasks {
		if t.Index < len(s.Results) || t.Index >= Slots(s) || t != task(s, t.Index) {
			return nil, fmt.Errorf("invalid task")
		}
	}
	for i, t := range tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			candidate := agent.Policy{ID: s.Population[t.Candidate].ID, Kind: "linear", Weights: s.Population[t.Candidate].Weights}
			opponent := s.Development[t.Opponent]
			factories := [2]arena.Factory{}
			factories[t.Side] = arena.Factory{ID: candidate.ID, New: candidate.New}
			factories[t.Side.Other()] = arena.Factory{ID: opponent.ID, New: opponent.New}
			r, err := arena.Play(ctx, arena.MatchConfig{Seed: t.Seed, ID: uint64(t.Index), StreamID: 0, MaxTurns: s.Config.MaxTurns}, factories)
			errs[i] = err
			if err == nil {
				scores[i] = Score{Index: t.Index, Seed: t.Seed, Side: t.Side, OpponentID: opponent.ID, Status: r.Record.Status, Outcome: r.Record.Outcome, Turns: r.Decisions, ForwardEvaluations: r.ForwardEvaluations}
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return scores, nil
}
func ValidScore(score Score, maxTurns int) error {
	if score.Turns < 1 || score.Turns > maxTurns || score.ForwardEvaluations < 0 || !score.Side.Valid() {
		return fmt.Errorf("invalid score counters")
	}
	switch score.Status {
	case replay.Completed:
		if score.Outcome == nil || !score.Outcome.Winner.Valid() || score.Outcome.Points != 1+boolInt(score.Outcome.Mars) {
			return fmt.Errorf("invalid completed result")
		}
	case replay.Truncated:
		if score.Outcome != nil || score.Turns != maxTurns {
			return fmt.Errorf("invalid truncated result")
		}
	default:
		return fmt.Errorf("invalid result status")
	}
	return nil
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func Add(s State, scores []Score) (State, error) {
	s.Results = append([]Score{}, s.Results...)
	truncated := false
	for _, score := range scores {
		t := task(s, len(s.Results))
		if score.Index != t.Index || score.Seed != t.Seed || score.Side != t.Side || score.OpponentID != s.Development[t.Opponent].ID || score.Index >= Slots(s) {
			return s, fmt.Errorf("result schedule mismatch")
		}
		if err := ValidScore(score, s.Config.MaxTurns); err != nil {
			return s, err
		}
		if score.Outcome != nil {
			o := *score.Outcome
			score.Outcome = &o
		}
		s.Results = append(s.Results, score)
		s.Counters.Games++
		if score.Status == replay.Completed {
			s.Counters.CompletedGames++
		} else {
			s.Counters.TruncatedGames++
		}
		s.Counters.Decisions += uint64(score.Turns)
		s.Counters.ForwardEvaluations += uint64(score.ForwardEvaluations)
		if score.Status == replay.Truncated {
			truncated = true
		}
	}
	if truncated {
		return s, ErrTruncated
	}
	return s, nil
}
func Ready(s State) bool {
	if len(s.Results) != Slots(s) || s.Generation >= s.Config.Generations {
		return false
	}
	for _, r := range s.Results {
		if r.Status != replay.Completed {
			return false
		}
	}
	return true
}
func Summarize(scores []Score) (Stats, error) {
	stats := Stats{}
	for _, r := range scores {
		if r.Status != replay.Completed || r.Outcome == nil {
			return stats, ErrTruncated
		}
		stats.Games++
		points := r.Outcome.Points
		if r.Outcome.Winner == r.Side {
			stats.Wins++
			if r.Outcome.Mars {
				stats.MarsWins++
			}
		} else {
			points = -points
		}
		stats.Points += points
	}
	if stats.Games == 0 {
		return stats, fmt.Errorf("no completed games")
	}
	stats.Fitness = float64(2*stats.Wins-stats.Games) / float64(stats.Games)
	return stats, nil
}
func Advance(s State) (State, Generation, error) {
	if !Ready(s) {
		return s, Generation{}, fmt.Errorf("generation evaluation is incomplete")
	}
	ranked := append([]Candidate{}, s.Population...)
	metric := Metric{Generation: s.Generation + 1, Games: len(s.Results)}
	for i := range ranked {
		n := s.Config.PairsPerOpponent * 4
		stats, err := Summarize(s.Results[i*n : (i+1)*n])
		if err != nil {
			return s, Generation{}, err
		}
		ranked[i].Stats = &stats
		metric.MeanFitness += stats.Fitness / float64(len(ranked))
	}
	for _, r := range s.Results {
		metric.Decisions += uint64(r.Turns)
		metric.ForwardEvaluations += uint64(r.ForwardEvaluations)
	}
	slices.SortStableFunc(ranked, func(a, b Candidate) int {
		if a.Stats.Fitness > b.Stats.Fitness {
			return -1
		}
		if a.Stats.Fitness < b.Stats.Fitness {
			return 1
		}
		return 0
	})
	metric.BestFitness = ranked[0].Stats.Fitness
	metric.BestCandidateID = ranked[0].ID
	gen := Generation{Algorithm: Algorithm, Ruleset: game.Ruleset, FeaturesVersion: features.Version, Number: s.Generation + 1, Ranked: ranked, Scores: append([]Score{}, s.Results...), Metric: metric}
	s.Generation++
	s.History = append(append([]Metric{}, s.History...), metric)
	s.Results = []Score{}
	if s.Generation == s.Config.Generations {
		s.Population = ranked
		return s, gen, nil
	}
	elite := max(1, int(math.Ceil(float64(len(ranked))*s.Config.EliteFraction)))
	population := make([]Candidate, len(ranked))
	selection := random.New(s.Config.Seed, "ga/selection", uint64(s.Generation))
	tournament := func() Candidate {
		best := selection.IntN(len(ranked))
		for i := 1; i < s.Config.TournamentSize; i++ {
			index := selection.IntN(len(ranked))
			if index < best {
				best = index
			}
		}
		return ranked[best]
	}
	for i := range population {
		child := Candidate{ID: candidateID(s.Generation, i), Parents: []string{}}
		if i < elite {
			child.Weights = ranked[i].Weights
			child.Parents = []string{ranked[i].ID}
		} else {
			a, b := tournament(), tournament()
			child.Parents = []string{a.ID, b.ID}
			crossover := random.New(s.Config.Seed, fmt.Sprintf("ga/crossover/%d", s.Generation), uint64(i))
			mutation := random.New(s.Config.Seed, fmt.Sprintf("ga/mutation/%d", s.Generation), uint64(i))
			for j := range child.Weights {
				w := a.Weights[j]
				if crossover.IntN(2) == 1 {
					w = b.Weights[j]
				}
				child.Weights[j] = bound(w + mutation.NormFloat64()*s.Config.MutationSigma)
				if child.Weights[j] != w {
					s.Counters.Mutations++
				}
			}
			s.Counters.Crossovers++
		}
		population[i] = child
	}
	s.Population = population
	return s, gen, nil
}
func Validate(s State) error {
	if s.Version != 1 || s.Algorithm != Algorithm || s.RandomContract != RandomContract || s.Ruleset != game.Ruleset || s.FeaturesVersion != features.Version {
		return fmt.Errorf("incompatible training checkpoint")
	}
	if err := s.Config.Validate(); err != nil {
		return err
	}
	if s.Generation < 0 || s.Generation > s.Config.Generations || len(s.History) != s.Generation || len(s.Population) != s.Config.Population || len(s.Results) > Slots(s) {
		return fmt.Errorf("invalid checkpoint dimensions")
	}
	for _, p := range s.Development {
		if err := p.Validate(); err != nil {
			return err
		}
	}
	if s.Development[0].ID != "builtin/heuristic-v1" || s.Development[1].ID != "builtin/random-v1" {
		return fmt.Errorf("incompatible development opponents")
	}
	seen := map[string]bool{}
	for _, p := range s.Population {
		if seen[p.ID] || p.ID == "" {
			return fmt.Errorf("duplicate candidate")
		}
		seen[p.ID] = true
		for _, w := range p.Weights {
			if math.IsNaN(w) || math.IsInf(w, 0) || math.Abs(w) > 10 {
				return fmt.Errorf("invalid genome")
			}
		}
	}
	var games, completed, truncated, decisions, forwards uint64
	for i, m := range s.History {
		if m.Generation != i+1 || m.Games != Slots(s) || math.IsNaN(m.BestFitness) || math.IsNaN(m.MeanFitness) || math.Abs(m.BestFitness) > 1 || math.Abs(m.MeanFitness) > 1 {
			return fmt.Errorf("invalid generation metrics")
		}
		games += uint64(m.Games)
		completed += uint64(m.Games)
		decisions += m.Decisions
		forwards += m.ForwardEvaluations
	}
	for i, r := range s.Results {
		t := task(s, i)
		if r.Index != i || r.Seed != t.Seed || r.Side != t.Side || r.OpponentID != s.Development[t.Opponent].ID {
			return fmt.Errorf("corrupt checkpoint schedule")
		}
		if err := ValidScore(r, s.Config.MaxTurns); err != nil {
			return err
		}
		games++
		if r.Status == replay.Completed {
			completed++
		} else {
			truncated++
		}
		decisions += uint64(r.Turns)
		forwards += uint64(r.ForwardEvaluations)
	}
	if games != s.Counters.Games || completed != s.Counters.CompletedGames || truncated != s.Counters.TruncatedGames || decisions != s.Counters.Decisions || forwards != s.Counters.ForwardEvaluations {
		return fmt.Errorf("checkpoint counters mismatch")
	}
	return nil
}
