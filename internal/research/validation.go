package research

import (
	"evonardy/internal/agent"
	"evonardy/internal/encoder"
	"evonardy/internal/game"
	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/neural"
	"evonardy/internal/training"
	"fmt"
	"math"
	"reflect"
	"time"
)

func validate(r record) error {
	if (r.FinalDataRole != "heldout" && r.FinalDataRole != "reused-development") || (r.FinalDataRole == "heldout" && len(r.FinalDataReusedFrom) > 0) || (r.FinalDataRole == "reused-development" && len(r.FinalDataReusedFrom) == 0) {
		return fmt.Errorf("invalid final data provenance")
	}
	if r.Format != 1 || !library.ValidCommandID(r.ID) || r.Version < 1 || r.Revision < r.Version || r.CreationHash == "" || r.Name == "" || len(r.Commands) > 1024 {
		return fmt.Errorf("invalid experiment envelope")
	}
	if err := r.Config.Validate(); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339Nano, r.CreatedAt); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339Nano, r.UpdatedAt); err != nil {
		return err
	}
	x := r.Execution
	if x.TDContract != training.TDRandomContract || x.PopulationContract != training.NeuroRandomContract || x.Ruleset != game.Ruleset || x.Encoder != encoder.Version || x.Network != neural.Version || x.Schedule != ScheduleVersion || x.Workers != r.Config.Workers || x.Platform == "" || x.GoVersion == "" || x.SourceRevision == "" {
		return fmt.Errorf("incompatible experiment execution contract")
	}
	if r.State != jobs.Queued && r.State != jobs.Running && r.State != jobs.Stopping && !terminal(r.State) {
		return fmt.Errorf("invalid experiment state")
	}
	if len(r.Opponents) != len(r.Config.OpponentIDs) || len(r.FrozenOpponents) != len(r.Opponents) || r.Incumbent.ID != r.Config.IncumbentID || identity(r.Incumbent) != r.FrozenIncumbent {
		return fmt.Errorf("frozen evaluation identities differ")
	}
	for i, p := range r.Opponents {
		if err := p.Validate(); err != nil {
			return err
		}
		if p.ID != r.Config.OpponentIDs[i] || identity(p) != r.FrozenOpponents[i] {
			return fmt.Errorf("opponent checksum mismatch")
		}
	}
	if err := r.Incumbent.Validate(); err != nil {
		return err
	}
	if err := validateSeeds(r.Runs, r.Config); err != nil {
		return err
	}
	n := len(r.Config.Methods) * len(r.Config.TrainingSeeds)
	if len(r.Runs) > n {
		return fmt.Errorf("excess runs")
	}
	var counters training.Counters
	wall := r.FinalSeconds
	for i, run := range r.Runs {
		if err := validateRun(run, r.Config, i, true); err != nil {
			return err
		}
		add(&counters, run.Counters)
		add(&counters, run.DevelopmentCounters)
		wall += run.WallSeconds + run.DevelopmentSeconds
	}
	if r.Active != nil {
		if len(r.Runs) >= n {
			return fmt.Errorf("unexpected active run")
		}
		if err := validateRun(*r.Active, r.Config, len(r.Runs), false); err != nil {
			return err
		}
		add(&counters, r.Active.Counters)
		add(&counters, r.Active.DevelopmentCounters)
		wall += r.Active.WallSeconds + r.Active.DevelopmentSeconds
	}
	if r.TD != nil {
		if r.Active == nil || r.Neuro != nil || r.Phase != "training" || r.Active.Config.TD == nil || *r.Active.Config.TD != r.TD.Config {
			return fmt.Errorf("TD config mismatch")
		}
		if err := training.ValidateTD(*r.TD); err != nil {
			return err
		}
		if r.Active.Counters != r.TD.Counters {
			return fmt.Errorf("TD counters mismatch")
		}
	}
	if r.Neuro != nil {
		if r.Active == nil || r.TD != nil || r.Phase != "training" {
			return fmt.Errorf("population phase mismatch")
		}
		if err := training.ValidateNeuro(*r.Neuro); err != nil {
			return err
		}
		if r.Active.Counters != r.Neuro.Counters {
			return fmt.Errorf("population counters mismatch")
		}
		cfg := r.Active.Config
		if cfg.GA != nil && *cfg.GA != r.Neuro.Config {
			return fmt.Errorf("GA config mismatch")
		}
		if cfg.Hybrid != nil && (r.Neuro.Hybrid == nil || *cfg.Hybrid != r.Neuro.Hybrid.Config) {
			return fmt.Errorf("Hybrid config mismatch")
		}
	}
	if r.Target != nil {
		if err := r.Target.Validate(); err != nil {
			return err
		}
		if r.SelectionLocked {
			if r.Target.ID != r.CandidateID {
				return fmt.Errorf("locked candidate mismatch")
			}
		} else if r.Active == nil || r.Active.BotID != r.Target.ID {
			return fmt.Errorf("development target mismatch")
		}
	}
	if r.SelectionLocked {
		if len(r.Runs) != n || r.Active != nil || r.CandidateID == "" || r.Target == nil {
			return fmt.Errorf("invalid final selection lock")
		}
		best := 0
		for i, run := range r.Runs {
			if run.Estimate.WinRate > r.Runs[best].Estimate.WinRate {
				best = i
			}
		}
		if r.CandidateID != r.Runs[best].BotID {
			return fmt.Errorf("candidate differs from development selection")
		}
	} else if len(r.FinalScores) > 0 || r.Verdict != nil || r.CandidateID != "" {
		return fmt.Errorf("final data precedes selection lock")
	}
	if err := validateScores(r.FinalScores, r.Config, []agent.Policy{r.Incumbent}, 0, true); err != nil {
		return err
	}
	var final training.Counters
	for _, s := range r.FinalScores {
		countScore(&final, s)
	}
	if final != r.FinalCounters {
		return fmt.Errorf("final counters mismatch")
	}
	add(&counters, final)
	if r.Counters != counters || math.IsNaN(wall) || math.IsInf(wall, 0) || wall < 0 || r.WallSeconds != wall {
		return fmt.Errorf("aggregate work mismatch")
	}
	if r.Verdict != nil {
		v, err := Confirm(r.FinalScores, r.Config.Confirmation)
		if err != nil {
			return err
		}
		if r.FinalDataRole == "reused-development" {
			v.Status = "candidate"
			v.Reason = "Final schedule was previously reserved; results are development data, not independent confirmation"
		}
		if !reflect.DeepEqual(v, *r.Verdict) {
			return fmt.Errorf("champion verdict differs from raw evidence")
		}
	}
	switch r.Phase {
	case "training":
		if r.Target != nil || r.SelectionLocked {
			return fmt.Errorf("invalid training phase")
		}
	case "development":
		if r.Active == nil || r.Target == nil || r.TD != nil || r.Neuro != nil || r.SelectionLocked {
			return fmt.Errorf("invalid development phase")
		}
	case "confirmation", "report", "completed":
		if !r.SelectionLocked {
			return fmt.Errorf("final phase without lock")
		}
	default:
		return fmt.Errorf("unknown phase")
	}
	if r.State == jobs.Completed && (r.Phase != "completed" || r.Verdict == nil || len(r.ReportSHA256) != 64) {
		return fmt.Errorf("incomplete experiment marked complete")
	}
	return nil
}
func validateRun(run Run, c Config, index int, complete bool) error {
	method := index / len(c.TrainingSeeds)
	seed := index % len(c.TrainingSeeds)
	if method >= len(c.Methods) || run.Seed != c.TrainingSeeds[seed] || run.Algorithm != c.Methods[method].Algorithm || !reflect.DeepEqual(run.Config, c.expanded(method, run.Seed)) {
		return fmt.Errorf("run config differs from cohort")
	}
	if run.TrainingGames+run.SelectionGames != run.Counters.Games || run.Counters.CompletedGames+run.Counters.TruncatedGames != run.Counters.Games || run.WallSeconds < 0 || run.DevelopmentSeconds < 0 || math.IsNaN(run.WallSeconds) || math.IsNaN(run.DevelopmentSeconds) || math.IsInf(run.WallSeconds, 0) || math.IsInf(run.DevelopmentSeconds, 0) {
		return fmt.Errorf("invalid run counters/time")
	}
	opponents := make([]agent.Policy, len(c.OpponentIDs))
	for i, id := range c.OpponentIDs {
		opponents[i].ID = id
	}
	if err := validateScores(run.Scores, c, opponents, seed, false); err != nil {
		return err
	}
	var counts training.Counters
	for _, s := range run.Scores {
		countScore(&counts, s)
	}
	if counts != run.DevelopmentCounters {
		return fmt.Errorf("development counters mismatch")
	}
	if complete {
		if run.BotID == "" || run.Manifest == nil || run.Manifest.ID != run.BotID || len(run.Scores) != c.DevelopmentPairs*len(c.OpponentIDs)*2 || run.Estimate == nil {
			return fmt.Errorf("incomplete seed run")
		}
		e, err := Analyze([][]training.Score{run.Scores}, c.Bootstrap)
		if err != nil {
			return err
		}
		if !reflect.DeepEqual(e, *run.Estimate) {
			return fmt.Errorf("run estimate differs from raw evidence")
		}
	}
	return nil
}
func validateScores(scores []training.Score, c Config, opponents []agent.Policy, seedIndex int, final bool) error {
	total := c.DevelopmentPairs * len(opponents) * 2
	if final {
		total = c.Confirmation.Pairs * 2
	}
	if len(scores) > total {
		return fmt.Errorf("evaluation exceeds quota")
	}
	for i, s := range scores {
		seed, side, opponent := scoreSchedule(c, opponents, i, seedIndex, final)
		if s.Index != i || s.Seed != seed || s.Side != side || s.OpponentID != opponent.ID {
			return fmt.Errorf("evaluation schedule mismatch")
		}
		if err := training.ValidScore(s, c.MaxTurns); err != nil {
			return err
		}
	}
	return nil
}
