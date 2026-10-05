package training

import (
	"context"
	"fmt"
	"slices"

	"evonardy/internal/random"
)

const HybridAlgorithm = "hybrid-sync-v1"
const HybridParticipants = 8

type Bounds struct {
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}
type TDHyperparameters struct {
	Alpha   float64 `json:"alpha"`
	Epsilon float64 `json:"epsilon"`
	Lambda  float64 `json:"lambda"`
}
type HybridConfig struct {
	Seed             uint64 `json:"seed"`
	Rounds           int    `json:"rounds"`
	GamesPerRound    int    `json:"games_per_round"`
	PairsPerOpponent int    `json:"pairs_per_opponent"`
	Workers          int    `json:"workers"`
	MaxTurns         int    `json:"max_turns"`
	TDHyperparameters
	AlphaBounds   Bounds `json:"alpha_bounds"`
	EpsilonBounds Bounds `json:"epsilon_bounds"`
	LambdaBounds  Bounds `json:"lambda_bounds"`
}

func DefaultHybridConfig() HybridConfig {
	return HybridConfig{Seed: 42, Rounds: 4, GamesPerRound: 8, PairsPerOpponent: 1, Workers: 2, MaxTurns: 1200, TDHyperparameters: TDHyperparameters{Alpha: .001, Epsilon: .05, Lambda: .7}, AlphaBounds: Bounds{.00001, .1}, EpsilonBounds: Bounds{0, .3}, LambdaBounds: Bounds{0, .95}}
}
func (c HybridConfig) selectionConfig() Config {
	return Config{Seed: c.Seed, Population: HybridParticipants, Generations: c.Rounds, PairsPerOpponent: c.PairsPerOpponent, Workers: c.Workers, MaxTurns: c.MaxTurns, EliteFraction: .25, TournamentSize: 2, InitialSigma: 1, MutationSigma: .02}
}
func (c HybridConfig) Validate() error {
	if err := c.selectionConfig().Validate(); err != nil {
		return err
	}
	games := int64(c.Rounds) * HybridParticipants * (int64(c.GamesPerRound) + int64(c.PairsPerOpponent)*4)
	if c.GamesPerRound < 1 || c.GamesPerRound > 1000 || games > 200000 || games*int64(c.MaxTurns) > 200000000 {
		return fmt.Errorf("invalid Hybrid budget: games_per_round 1..1000; up to 200000 total games and 200000000 turn slots")
	}
	for _, v := range []struct {
		b     Bounds
		x     float64
		alpha bool
	}{{c.AlphaBounds, c.Alpha, true}, {c.EpsilonBounds, c.Epsilon, false}, {c.LambdaBounds, c.Lambda, false}} {
		if !finiteTD(v.b.Min) || !finiteTD(v.b.Max) || !finiteTD(v.x) || v.b.Min < 0 || v.b.Max > 1 || v.b.Min >= v.b.Max || v.x < v.b.Min || v.x > v.b.Max || (v.alpha && v.b.Min <= 0) {
			return fmt.Errorf("invalid Hybrid hyperparameters or explicit bounds")
		}
	}
	return nil
}

type Replacement struct {
	ChildID    string            `json:"child_id"`
	ParentID   string            `json:"parent_id"`
	ReplacedID string            `json:"replaced_id"`
	Reason     string            `json:"reason"`
	Before     TDHyperparameters `json:"before"`
	After      TDHyperparameters `json:"after"`
}
type HybridProgress struct {
	Config        HybridConfig `json:"config"`
	TrainingDone  []int        `json:"training_done"`
	RoundCounters Counters     `json:"round_counters"`
	History       []Counters   `json:"history"`
}

func NewHybrid(c HybridConfig) (NeuroState, error) {
	if err := c.Validate(); err != nil {
		return NeuroState{}, err
	}
	s, err := NewGAMLP(c.selectionConfig())
	if err != nil {
		return s, err
	}
	s.Algorithm = HybridAlgorithm
	s.Hybrid = &HybridProgress{Config: c, TrainingDone: make([]int, HybridParticipants), History: []Counters{}}
	for i := range s.Population {
		s.Population[i].Hyperparameters = c.TDHyperparameters
	}
	return s, nil
}
func cloneHybrid(s NeuroState) NeuroState {
	h := *s.Hybrid
	h.TrainingDone = slices.Clone(h.TrainingDone)
	h.History = slices.Clone(h.History)
	s.Hybrid = &h
	s.Population = slices.Clone(s.Population)
	for i := range s.Population {
		s.Population[i].Parents = slices.Clone(s.Population[i].Parents)
		if s.Population[i].Stats != nil {
			stats := *s.Population[i].Stats
			s.Population[i].Stats = &stats
		}
	}
	return s
}
func HybridTrainingReady(s NeuroState) bool {
	if s.Hybrid == nil {
		return false
	}
	for _, n := range s.Hybrid.TrainingDone {
		if n != s.Hybrid.Config.GamesPerRound {
			return false
		}
	}
	return true
}

// Each participant owns a sequential learner. Checkpoints are game boundaries,
// so TrainTDGame starts with empty traces, including after replacement/resume.
func TrainHybridGame(ctx context.Context, s NeuroState) (NeuroState, PlayedGame, error) {
	if err := ValidateNeuro(s); err != nil {
		return s, PlayedGame{}, err
	}
	if s.Algorithm != HybridAlgorithm || s.Generation >= s.Config.Generations || HybridTrainingReady(s) || len(s.Results) > 0 {
		return s, PlayedGame{}, fmt.Errorf("Hybrid training phase unavailable")
	}
	index := 0
	for s.Hybrid.TrainingDone[index] == s.Hybrid.Config.GamesPerRound {
		index++
	}
	p := s.Population[index]
	count := s.Hybrid.TrainingDone[index]
	seed := random.New(s.Config.Seed, fmt.Sprintf("hybrid/round/%d/participant/%d", s.Generation, index), uint64(count)).Uint64()
	td, err := NewTD(TDConfig{Seed: seed, Games: 1, MaxTurns: s.Config.MaxTurns, Alpha: p.Hyperparameters.Alpha, Epsilon: p.Hyperparameters.Epsilon, Lambda: p.Hyperparameters.Lambda})
	if err != nil {
		return s, PlayedGame{}, err
	}
	td.Parameters = p.Parameters
	td, played, err := TrainTDGame(ctx, td)
	if err != nil {
		return s, played, err
	}
	next := cloneHybrid(s)
	next.Population[index].Parameters = td.Parameters
	next.Hybrid.TrainingDone[index]++
	next.Hybrid.RoundCounters = addCounters(next.Hybrid.RoundCounters, td.Counters)
	next.Counters = addCounters(next.Counters, td.Counters)
	played.Generation = s.Generation + 1
	played.Index = index*s.Hybrid.Config.GamesPerRound + count
	played.CandidateID = p.ID
	played.OpponentID = p.ID
	played.Replay.Bots = [2]string{p.ID, p.ID}
	return next, played, nil
}
func addCounters(a, b Counters) Counters {
	a.Games += b.Games
	a.CompletedGames += b.CompletedGames
	a.TruncatedGames += b.TruncatedGames
	a.Decisions += b.Decisions
	a.ForwardEvaluations += b.ForwardEvaluations
	a.Mutations += b.Mutations
	a.Crossovers += b.Crossovers
	a.Updates += b.Updates
	return a
}
func hybridTrainingCounters(s NeuroState) Counters {
	total := s.Hybrid.RoundCounters
	for _, c := range s.Hybrid.History {
		total = addCounters(total, c)
	}
	return total
}
func AdvanceHybrid(s NeuroState) (NeuroState, NeuroGeneration, error) {
	if !HybridTrainingReady(s) {
		return s, NeuroGeneration{}, fmt.Errorf("Hybrid training incomplete")
	}
	gen, err := RankNeuro(s)
	if err != nil {
		return s, gen, err
	}
	gen.TrainingCounters = s.Hybrid.RoundCounters
	next := cloneHybrid(s)
	next.Generation++
	next.History = append(slices.Clone(s.History), gen.Metric)
	next.Results = []Score{}
	next.Hybrid.History = append(next.Hybrid.History, s.Hybrid.RoundCounters)
	next.Hybrid.RoundCounters = Counters{}
	next.Hybrid.TrainingDone = make([]int, HybridParticipants)
	if next.Generation == s.Config.Generations {
		next.Population = gen.Ranked
		return next, gen, nil
	}
	ranked := gen.Ranked
	population := make([]NeuroCandidate, HybridParticipants)
	for i := range population {
		parent := i
		reason := "survivor"
		if i >= HybridParticipants*3/4 {
			parent = i - HybridParticipants*3/4
			reason = "bottom-quarter fitness replacement"
		}
		original := ranked[i]
		a := ranked[parent]
		child := NeuroCandidate{ID: neuroID(next.Generation, i), Parameters: a.Parameters, Hyperparameters: a.Hyperparameters, Parents: []string{a.ID}, Reason: reason}
		if parent != i {
			r := random.New(s.Config.Seed, "hybrid/hyperparameters", uint64(next.Generation*HybridParticipants+i))
			factor := .8
			if r.IntN(2) == 1 {
				factor = 1.2
			}
			c := s.Hybrid.Config
			perturb := func(x, step float64, b Bounds) float64 {
				if r.IntN(2) == 0 {
					step = -step
				}
				return max(b.Min, min(b.Max, x+step))
			}
			child.Hyperparameters.Alpha = max(c.AlphaBounds.Min, min(c.AlphaBounds.Max, a.Hyperparameters.Alpha*factor))
			child.Hyperparameters.Epsilon = perturb(a.Hyperparameters.Epsilon, .02, c.EpsilonBounds)
			child.Hyperparameters.Lambda = perturb(a.Hyperparameters.Lambda, .05, c.LambdaBounds)
			gen.Replacements = append(gen.Replacements, Replacement{ChildID: child.ID, ParentID: a.ID, ReplacedID: original.ID, Reason: reason, Before: a.Hyperparameters, After: child.Hyperparameters})
		}
		population[i] = child
	}
	next.Population = population
	return next, gen, nil
}
func validateHybrid(s NeuroState) error {
	if s.Hybrid == nil {
		return fmt.Errorf("missing Hybrid state")
	}
	h := s.Hybrid
	c := h.Config
	if err := c.Validate(); err != nil {
		return err
	}
	if s.Config != c.selectionConfig() || len(h.TrainingDone) != HybridParticipants || len(h.History) != s.Generation || s.Counters.Mutations != 0 || s.Counters.Crossovers != 0 {
		return fmt.Errorf("invalid Hybrid dimensions")
	}
	var games int
	prefixEnded := false
	for _, n := range h.TrainingDone {
		if prefixEnded && n != 0 {
			return fmt.Errorf("invalid Hybrid participant prefix")
		}
		if n < c.GamesPerRound {
			prefixEnded = true
		}
		if n < 0 || n > c.GamesPerRound {
			return fmt.Errorf("invalid participant progress")
		}
		games += n
	}
	valid := func(work Counters, n int) bool {
		return work.Games == uint64(n) && work.CompletedGames+work.TruncatedGames == work.Games && work.Updates == work.Decisions && work.Decisions >= work.CompletedGames+work.TruncatedGames*uint64(c.MaxTurns) && work.Decisions <= work.Games*uint64(c.MaxTurns) && work.ForwardEvaluations >= work.Updates && work.Mutations == 0 && work.Crossovers == 0
	}
	if !valid(h.RoundCounters, games) || (len(s.Results) > 0 && !HybridTrainingReady(s)) {
		return fmt.Errorf("invalid Hybrid training counters or phase")
	}
	for _, work := range h.History {
		if !valid(work, HybridParticipants*c.GamesPerRound) {
			return fmt.Errorf("invalid Hybrid round history")
		}
	}
	for _, p := range s.Population {
		x := p.Hyperparameters
		if !finiteTD(x.Alpha) || !finiteTD(x.Epsilon) || !finiteTD(x.Lambda) || x.Alpha < c.AlphaBounds.Min || x.Alpha > c.AlphaBounds.Max || x.Epsilon < c.EpsilonBounds.Min || x.Epsilon > c.EpsilonBounds.Max || x.Lambda < c.LambdaBounds.Min || x.Lambda > c.LambdaBounds.Max {
			return fmt.Errorf("Hybrid participant outside bounds")
		}
	}
	if hybridTrainingCounters(s).Updates != s.Counters.Updates {
		return fmt.Errorf("missing discarded Hybrid updates")
	}
	return nil
}

// StepNeuro performs one safe boundary: a learner game or a selection wave.
func StepNeuro(ctx context.Context, s NeuroState) (NeuroState, *NeuroGeneration, *PlayedGame, error) {
	if err := ValidateNeuro(s); err != nil {
		return s, nil, nil, err
	}
	if s.Hybrid != nil && !HybridTrainingReady(s) {
		next, played, err := TrainHybridGame(ctx, s)
		if err != nil {
			return s, nil, nil, err
		}
		return next, nil, &played, nil
	}
	wave, err := PlayNeuroWave(ctx, s)
	if err != nil {
		return s, nil, nil, err
	}
	if s.Hybrid != nil && wave.Sample != nil {
		wave.Sample.Index += HybridParticipants * s.Hybrid.Config.GamesPerRound
	}
	next, err := AddNeuro(s, wave.Scores)
	if err != nil {
		return next, nil, wave.Sample, err
	}
	if NeuroReady(next) {
		var gen NeuroGeneration
		if s.Hybrid != nil {
			next, gen, err = AdvanceHybrid(next)
		} else {
			next, gen, err = AdvanceNeuro(next)
		}
		return next, &gen, wave.Sample, err
	}
	return next, nil, wave.Sample, nil
}
