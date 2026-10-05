// Package study orchestrates a bounded local comparison using durable jobs.
package study

import (
	"fmt"
	"time"

	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/research"
	"evonardy/internal/training"
)

const Version = "local-strength-study-v1"

type Method struct {
	Name    string            `json:"name"`
	Request jobs.StartRequest `json:"request"`
}

type Config struct {
	Version               string                        `json:"version"`
	Seeds                 []uint64                      `json:"training_seeds"`
	Methods               []Method                      `json:"methods"`
	DevelopmentPairs      int                           `json:"development_pairs"`
	RandomPairs           int                           `json:"random_pairs"`
	TournamentPairs       int                           `json:"tournament_pairs"`
	Workers               int                           `json:"workers"`
	MaxTurns              int                           `json:"max_turns"`
	TimeLimitSeconds      int                           `json:"time_limit_seconds"`
	TrainingCutoffSeconds int                           `json:"training_cutoff_seconds"`
	Bootstrap             research.Bootstrap            `json:"bootstrap"`
	Confirmation          research.ConfirmationProtocol `json:"confirmation"`
}

func DefaultConfig() Config {
	linear := training.DefaultConfig()
	linear.Generations = 8
	mlp := training.DefaultGAMLPConfig()
	mlp.Population, mlp.Generations = 16, 32
	td := training.DefaultTDConfig()
	td.Games = 4096
	lambda := training.DefaultTDLambdaConfig()
	lambda.Games = 4096
	hybrid := training.DefaultHybridConfig()
	hybrid.Rounds, hybrid.GamesPerRound, hybrid.PairsPerOpponent = 8, 56, 2
	confirmation := research.DefaultConfirmation()
	confirmation.Pairs, confirmation.MinimumPairs = 500, 500
	return Config{Version: Version, Seeds: []uint64{1001, 1002, 1003, 1004, 1005}, Methods: []Method{
		{"ga-linear", jobs.StartRequest{Config: linear, Algorithm: training.Algorithm}},
		{"ga-mlp", jobs.StartRequest{Config: mlp, Algorithm: training.GAMLPAlgorithm}},
		{"td0", jobs.StartRequest{TDConfig: &td, Algorithm: td.Algorithm()}},
		{"td-lambda", jobs.StartRequest{TDConfig: &lambda, Algorithm: lambda.Algorithm()}},
		{"hybrid", jobs.StartRequest{HybridConfig: &hybrid, Algorithm: training.HybridAlgorithm}},
	}, DevelopmentPairs: 50, RandomPairs: 50, TournamentPairs: 100, Workers: 2, MaxTurns: 1200, TimeLimitSeconds: 7200, TrainingCutoffSeconds: 6300, Bootstrap: research.DefaultBootstrap(), Confirmation: confirmation}
}

func (c Config) Validate() error {
	if c.Version != Version || len(c.Seeds) < 1 || len(c.Seeds) > 20 || len(c.Methods) < 1 || len(c.Methods) > 5 || c.TimeLimitSeconds < 1 || c.TrainingCutoffSeconds < 1 || c.TrainingCutoffSeconds >= c.TimeLimitSeconds || c.TimeLimitSeconds > 86400 {
		return fmt.Errorf("invalid study cohort or time budget")
	}
	seen := map[uint64]bool{}
	for _, seed := range c.Seeds {
		if seen[seed] {
			return fmt.Errorf("duplicate training seed")
		}
		seen[seed] = true
	}
	names := map[string]bool{}
	var games, turns int64
	for _, method := range c.Methods {
		if names[method.Name] || method.Name == "" {
			return fmt.Errorf("duplicate or missing method")
		}
		names[method.Name] = true
		r := method.Request
		if r.CommandID != "" || r.ExpectedVersion != 0 || r.Name != "" || r.SourceBotID != "" {
			return fmt.Errorf("study method requests must not contain job controls or saved-source training")
		}
		var n int64
		switch method.Name {
		case "ga-linear", "ga-mlp":
			want := training.Algorithm
			if method.Name == "ga-mlp" {
				want = training.GAMLPAlgorithm
			}
			if r.Algorithm != want || r.TDConfig != nil || r.HybridConfig != nil {
				return fmt.Errorf("invalid GA method")
			}
			if err := r.Config.Validate(); err != nil {
				return err
			}
			if method.Name == "ga-mlp" && r.Config.InitialSigma != 1 {
				return fmt.Errorf("neural initialization scale must be one")
			}
			n = int64(r.Config.Population * r.Config.Generations * r.Config.PairsPerOpponent * 4)
		case "td0", "td-lambda":
			if r.TDConfig == nil || r.Config != (training.Config{}) || r.HybridConfig != nil || r.Algorithm != r.TDConfig.Algorithm() || r.TDConfig.Games > 10000 {
				return fmt.Errorf("invalid TD method")
			}
			if (method.Name == "td0") != (r.TDConfig.Lambda == 0) {
				return fmt.Errorf("TD lambda mismatch")
			}
			if err := r.TDConfig.Validate(); err != nil {
				return err
			}
			n = int64(r.TDConfig.Games)
		case "hybrid":
			if r.HybridConfig == nil || r.Config != (training.Config{}) || r.TDConfig != nil || r.Algorithm != training.HybridAlgorithm {
				return fmt.Errorf("invalid Hybrid method")
			}
			if err := r.HybridConfig.Validate(); err != nil {
				return err
			}
			h := r.HybridConfig
			n = int64(h.Rounds * training.HybridParticipants * (h.GamesPerRound + h.PairsPerOpponent*4))
		default:
			return fmt.Errorf("unsupported study method")
		}
		games += n * int64(len(c.Seeds))
		guard := r.Config.MaxTurns
		if r.TDConfig != nil {
			guard = r.TDConfig.MaxTurns
		}
		if r.HybridConfig != nil {
			guard = r.HybridConfig.MaxTurns
		}
		turns += n * int64(len(c.Seeds)*guard)
	}
	for _, pairs := range []int{c.DevelopmentPairs, c.RandomPairs, c.TournamentPairs, c.Confirmation.Pairs} {
		if err := (jobs.EvaluationConfig{Pairs: pairs, Workers: c.Workers, MaxTurns: c.MaxTurns, OpponentIDs: []string{library.HeuristicID}}).Validate(); err != nil {
			return err
		}
	}
	if c.DevelopmentPairs*4 > 1000 {
		return fmt.Errorf("development cohort exceeds statistics capacity")
	}
	finalists := int64(len(c.Methods) + 2)
	if int64(len(c.Methods)*len(c.Seeds)*3)+finalists*(finalists-1)/2+1 > 200 {
		return fmt.Errorf("study exceeds 200 durable jobs")
	}
	evalGames := int64(len(c.Methods)*len(c.Seeds)*(c.DevelopmentPairs*4+c.RandomPairs*2)) + finalists*(finalists-1)*int64(c.TournamentPairs) + int64(c.Confirmation.Pairs*2)
	if games+evalGames > 200000 || turns+evalGames*int64(c.MaxTurns) > 200000000 {
		return fmt.Errorf("study exceeds 200000 physical games or 200000000 turn slots")
	}
	if err := c.Bootstrap.Validate(); err != nil {
		return err
	}
	if int64(len(c.Seeds))*int64(c.DevelopmentPairs)*2*int64(c.Bootstrap.Resamples) > 20000000 {
		return fmt.Errorf("development bootstrap exceeds 20000000 pair draws")
	}
	return c.Confirmation.Validate()
}

func (c Config) duration() time.Duration { return time.Duration(c.TimeLimitSeconds) * time.Second }

func expanded(method Method, seed uint64) jobs.StartRequest {
	r := method.Request
	if r.TDConfig != nil {
		x := *r.TDConfig
		x.Seed = seed
		r.TDConfig = &x
	} else if r.HybridConfig != nil {
		x := *r.HybridConfig
		x.Seed = seed
		r.HybridConfig = &x
	} else {
		r.Config.Seed = seed
	}
	r.Name = fmt.Sprintf("Study %s seed %d", method.Name, seed)
	return r
}
