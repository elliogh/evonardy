package research

import (
	"evonardy/internal/library"
	"evonardy/internal/training"
	"fmt"
)

const ExperimentVersion = "research-experiment-v1"
const ScheduleVersion = "research-phase-paired-v1"

type Method struct {
	Algorithm string                 `json:"algorithm"`
	GA        *training.Config       `json:"ga,omitempty"`
	TD        *training.TDConfig     `json:"td,omitempty"`
	Hybrid    *training.HybridConfig `json:"hybrid,omitempty"`
}
type Config struct {
	Version          string               `json:"version"`
	TrainingSeeds    []uint64             `json:"training_seeds"`
	Methods          []Method             `json:"methods"`
	DevelopmentSeed  uint64               `json:"development_seed"`
	FinalSeed        uint64               `json:"final_seed"`
	DevelopmentPairs int                  `json:"development_pairs"`
	OpponentIDs      []string             `json:"opponent_ids"`
	IncumbentID      string               `json:"incumbent_id"`
	Workers          int                  `json:"workers"`
	MaxTurns         int                  `json:"max_turns"`
	Bootstrap        Bootstrap            `json:"bootstrap"`
	Confirmation     ConfirmationProtocol `json:"confirmation"`
}

func DefaultConfig() Config {
	ga := training.DefaultGAMLPConfig()
	td := training.DefaultTDLambdaConfig()
	hy := training.DefaultHybridConfig()
	return Config{Version: ExperimentVersion, TrainingSeeds: []uint64{42, 43, 44, 45, 46}, Methods: []Method{{Algorithm: "ga-mlp", GA: &ga}, {Algorithm: "td-lambda", TD: &td}, {Algorithm: "hybrid", Hybrid: &hy}}, DevelopmentSeed: 2026, FinalSeed: 2027, DevelopmentPairs: 20, OpponentIDs: []string{library.HeuristicID, library.RandomID}, IncumbentID: library.HeuristicID, Workers: 2, MaxTurns: 1200, Bootstrap: DefaultBootstrap(), Confirmation: DefaultConfirmation()}
}
func (m Method) budget() (games, turns int64, err error) {
	switch m.Algorithm {
	case "ga-mlp":
		if m.GA == nil || m.TD != nil || m.Hybrid != nil {
			return 0, 0, fmt.Errorf("ga-mlp requires only ga config")
		}
		if err = m.GA.Validate(); err != nil {
			return
		}
		if m.GA.InitialSigma != 1 {
			return 0, 0, fmt.Errorf("research uses the shared neural initialization scale 1")
		}
		games = int64(m.GA.Population * m.GA.Generations * m.GA.PairsPerOpponent * 4)
		turns = games * int64(m.GA.MaxTurns)
	case "td0", "td-lambda":
		if m.TD == nil || m.GA != nil || m.Hybrid != nil {
			return 0, 0, fmt.Errorf("TD requires only td config")
		}
		if err = m.TD.Validate(); err != nil {
			return
		}
		if m.TD.Games > 10000 || (m.Algorithm == "td0" && m.TD.Lambda != 0) || (m.Algorithm == "td-lambda" && m.TD.Lambda == 0) {
			return 0, 0, fmt.Errorf("TD method or 10000-game boundary invalid")
		}
		games = int64(m.TD.Games)
		turns = games * int64(m.TD.MaxTurns)
	case "hybrid":
		if m.Hybrid == nil || m.GA != nil || m.TD != nil {
			return 0, 0, fmt.Errorf("hybrid requires only hybrid config")
		}
		if err = m.Hybrid.Validate(); err != nil {
			return
		}
		games = int64(m.Hybrid.Rounds * training.HybridParticipants * (m.Hybrid.GamesPerRound + m.Hybrid.PairsPerOpponent*4))
		turns = games * int64(m.Hybrid.MaxTurns)
	default:
		return 0, 0, fmt.Errorf("unsupported research method")
	}
	return
}
func (c Config) Validate() error {
	if c.Version != ExperimentVersion || len(c.TrainingSeeds) < 1 || len(c.TrainingSeeds) > 20 || len(c.Methods) < 1 || len(c.Methods) > 4 || c.DevelopmentSeed == c.FinalSeed || c.DevelopmentPairs < 1 || c.DevelopmentPairs > 250 || len(c.OpponentIDs) < 1 || len(c.OpponentIDs) > 8 || c.IncumbentID == "" || c.Workers < 1 || c.Workers > 8 || c.MaxTurns < 1 || c.MaxTurns > 10000 {
		return fmt.Errorf("invalid research configuration or phase separation")
	}
	seeds := map[uint64]bool{}
	for _, s := range c.TrainingSeeds {
		if seeds[s] || s == c.DevelopmentSeed || s == c.FinalSeed {
			return fmt.Errorf("training seeds must be unique and distinct from evaluation roots")
		}
		seeds[s] = true
	}
	names := map[string]bool{}
	for _, id := range c.OpponentIDs {
		if id == "" || names[id] {
			return fmt.Errorf("missing or duplicate opponent")
		}
		names[id] = true
	}
	if err := c.Bootstrap.Validate(); err != nil {
		return err
	}
	if err := c.Confirmation.Validate(); err != nil {
		return err
	}
	draws := int64(len(c.TrainingSeeds)*c.DevelopmentPairs*len(c.OpponentIDs)) * int64(c.Bootstrap.Resamples)
	if draws > 20000000 {
		return fmt.Errorf("research bootstrap exceeds 20000000 pair draws")
	}
	games := int64(len(c.Methods)*len(c.TrainingSeeds)*c.DevelopmentPairs*len(c.OpponentIDs)*2 + c.Confirmation.Pairs*2)
	turns := games * int64(c.MaxTurns)
	if c.DevelopmentPairs*len(c.OpponentIDs)*2 > 1000 || games > 10000 {
		return fmt.Errorf("research evaluation exceeds 10000 games or 1000 per run")
	}
	names = map[string]bool{}
	for _, m := range c.Methods {
		if names[m.Algorithm] {
			return fmt.Errorf("duplicate method")
		}
		names[m.Algorithm] = true
		g, t, err := m.budget()
		if err != nil {
			return err
		}
		games += g * int64(len(c.TrainingSeeds))
		turns += t * int64(len(c.TrainingSeeds))
	}
	if games > 200000 || turns > 200000000 {
		return fmt.Errorf("experiment exceeds 200000 physical games or 200000000 turn slots")
	}
	return nil
}
func (c Config) expanded(method int, seed uint64) Method {
	m := c.Methods[method]
	if m.GA != nil {
		v := *m.GA
		v.Seed = seed
		m.GA = &v
	}
	if m.TD != nil {
		v := *m.TD
		v.Seed = seed
		m.TD = &v
	}
	if m.Hybrid != nil {
		v := *m.Hybrid
		v.Seed = seed
		m.Hybrid = &v
	}
	return m
}
func validateSeeds(runs []Run, c Config) error {
	for i, r := range runs {
		method := i / len(c.TrainingSeeds)
		seed := i % len(c.TrainingSeeds)
		if method >= len(c.Methods) || r.Algorithm != c.Methods[method].Algorithm || r.Seed != c.TrainingSeeds[seed] {
			return fmt.Errorf("run order differs from declared cohort")
		}
	}
	return nil
}
