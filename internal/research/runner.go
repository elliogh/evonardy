package research

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"evonardy/internal/agent"
	"evonardy/internal/arena"
	"evonardy/internal/game"
	"evonardy/internal/jobs"
	"evonardy/internal/replay"
	"evonardy/internal/training"
)

func (m *Manager) step(r *record) error {
	switch r.Phase {
	case "training":
		if r.Active == nil {
			i := len(r.Runs)
			if i == len(r.Config.Methods)*len(r.Config.TrainingSeeds) {
				return m.selectCandidate(r)
			}
			seed := r.Config.TrainingSeeds[i%len(r.Config.TrainingSeeds)]
			cfg := r.Config.expanded(i/len(r.Config.TrainingSeeds), seed)
			r.Active = &Run{Algorithm: cfg.Algorithm, Seed: seed, Config: cfg, Scores: []training.Score{}}
			var err error
			if cfg.TD != nil {
				s, e := training.NewTD(*cfg.TD)
				err = e
				r.TD = &s
			} else {
				var s training.NeuroState
				if cfg.GA != nil {
					s, err = training.NewGAMLP(*cfg.GA)
				} else {
					s, err = training.NewHybrid(*cfg.Hybrid)
				}
				r.Neuro = &s
			}
			return err
		}
		start := time.Now()
		var parameters agent.NeuralParameters
		done := false
		var err error
		if r.TD != nil {
			s, _, e := training.TrainTDGame(context.Background(), *r.TD)
			err = e
			r.TD = &s
			r.Active.Counters = s.Counters
			r.Active.TrainingGames = s.Counters.Games
			done = s.Games == s.Config.Games
			parameters = s.Parameters
		} else {
			s, _, _, e := training.StepNeuro(context.Background(), *r.Neuro)
			err = e
			r.Neuro = &s
			r.Active.Counters = s.Counters
			r.Active.TrainingGames = 0
			if s.Hybrid != nil {
				r.Active.TrainingGames = s.Hybrid.RoundCounters.Games
				for _, c := range s.Hybrid.History {
					r.Active.TrainingGames += c.Games
				}
			}
			r.Active.SelectionGames = s.Counters.Games - r.Active.TrainingGames
			done = s.Generation == s.Config.Generations
			if done {
				parameters = s.Population[0].Parameters
			}
		}
		r.Active.WallSeconds += time.Since(start).Seconds()
		if err != nil {
			return err
		}
		if done {
			card, err := m.bots.SaveNeural(fmt.Sprintf("%s · seed %d", r.Active.Algorithm, r.Active.Seed), parameters[:], "Research "+r.ID, r.ID)
			if err != nil {
				return err
			}
			detail, err := m.bots.Get(card.ID)
			if err != nil {
				return err
			}
			p, err := m.bots.Freeze(card.ID)
			if err != nil {
				return err
			}
			r.Active.BotID = card.ID
			r.Active.Manifest = detail.Manifest
			r.Target = &p
			r.TD = nil
			r.Neuro = nil
			r.Phase = "development"
		}
		return nil
	case "development":
		start := time.Now()
		scores, err := playWave(r, false)
		r.Active.DevelopmentSeconds += time.Since(start).Seconds()
		if err != nil {
			return err
		}
		r.Active.Scores = append(r.Active.Scores, scores...)
		for _, s := range scores {
			countScore(&r.Active.DevelopmentCounters, s)
		}
		for _, s := range scores {
			if s.Status != replay.Completed {
				return fmt.Errorf("development game truncated; no estimate or final selection")
			}
		}
		if len(r.Active.Scores) == r.Config.DevelopmentPairs*len(r.Opponents)*2 {
			e, err := Analyze([][]training.Score{r.Active.Scores}, r.Config.Bootstrap)
			if err != nil {
				return err
			}
			r.Active.Estimate = &e
			r.Runs = append(r.Runs, *r.Active)
			r.Active = nil
			r.Target = nil
			r.Phase = "training"
		}
		return nil
	case "confirmation":
		if !r.SelectionLocked || r.Target == nil {
			return fmt.Errorf("candidate must be locked before final evaluation")
		}
		start := time.Now()
		scores, err := playWave(r, true)
		r.FinalSeconds += time.Since(start).Seconds()
		if err != nil {
			return err
		}
		r.FinalScores = append(r.FinalScores, scores...)
		for _, s := range scores {
			countScore(&r.FinalCounters, s)
		}
		for _, s := range scores {
			if s.Status != replay.Completed {
				return fmt.Errorf("final game truncated; no champion verdict")
			}
		}
		if len(r.FinalScores) == r.Config.Confirmation.Pairs*2 {
			v, err := Confirm(r.FinalScores, r.Config.Confirmation)
			if err != nil {
				return err
			}
			if r.FinalDataRole == "reused-development" {
				v.Status = "candidate"
				v.Reason = "Final schedule was previously reserved; results are development data, not independent confirmation"
			}
			r.Verdict = &v
			r.Phase = "report"
		}
		return nil
	case "report":
		r.State = jobs.Completed
		r.Phase = "completed"
		data, err := json.MarshalIndent(r.Snapshot, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		sha := hash(data)
		err = m.store.WriteNew(reportPath(r.ID, sha), data)
		if err != nil && !os.IsExist(err) {
			return err
		}
		r.ReportSHA256 = sha
		return nil
	default:
		return fmt.Errorf("unknown experiment phase")
	}
}
func countScore(c *training.Counters, s training.Score) {
	c.Games++
	c.Decisions += uint64(s.Turns)
	c.ForwardEvaluations += uint64(s.ForwardEvaluations)
	if s.Status == replay.Completed {
		c.CompletedGames++
	} else {
		c.TruncatedGames++
	}
}
func (m *Manager) selectCandidate(r *record) error {
	groups := make([][][]training.Score, len(r.Config.Methods))
	best := 0
	for i, run := range r.Runs {
		group := i / len(r.Config.TrainingSeeds)
		groups[group] = append(groups[group], run.Scores)
		if run.Estimate.WinRate > r.Runs[best].Estimate.WinRate {
			best = i
		}
	}
	for i, group := range groups {
		e, err := Analyze(group, r.Config.Bootstrap)
		if err != nil {
			return err
		}
		r.Methods = append(r.Methods, MethodResult{Algorithm: r.Config.Methods[i].Algorithm, Estimate: e})
	}
	for i := range groups {
		for j := i + 1; j < len(groups); j++ {
			e, err := Compare(groups[i], groups[j], r.Config.Bootstrap)
			if err != nil {
				return err
			}
			r.Differences = append(r.Differences, Difference{A: r.Config.Methods[i].Algorithm, B: r.Config.Methods[j].Algorithm, Estimate: e})
		}
	}
	p, err := m.bots.Freeze(r.Runs[best].BotID)
	if err != nil {
		return err
	}
	r.CandidateID = p.ID
	r.Target = &p
	r.SelectionLocked = true
	r.Phase = "confirmation"
	return nil
}
func scoreSchedule(c Config, opponents []agent.Policy, index int, seedIndex int, final bool) (uint64, game.Player, agent.Policy) {
	root, pairs, label := c.DevelopmentSeed, c.DevelopmentPairs, fmt.Sprintf("%s/development/seed/%d", ScheduleVersion, seedIndex)
	if final {
		root, pairs, label = c.FinalSeed, c.Confirmation.Pairs, ScheduleVersion+"/final"
	}
	opponent := index / (pairs * 2)
	pair := (index / 2) % pairs
	return training.PairSeed(root, label, opponent, pair), game.Player(index % 2), opponents[opponent]
}
func playWave(r *record, final bool) ([]training.Score, error) {
	index, total, opponents := len(r.ActiveScores(final)), r.Config.DevelopmentPairs*len(r.Opponents)*2, r.Opponents
	if final {
		total = r.Config.Confirmation.Pairs * 2
		opponents = []agent.Policy{r.Incumbent}
	}
	n := min(r.Config.Workers, total-index)
	if n < 1 {
		return nil, fmt.Errorf("evaluation budget exhausted")
	}
	scores := make([]training.Score, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range scores {
		wg.Add(1)
		go func() {
			defer wg.Done()
			slot := index + i
			seed, side, opponent := scoreSchedule(r.Config, opponents, slot, len(r.Runs)%len(r.Config.TrainingSeeds), final)
			factories := [2]arena.Factory{}
			factories[side] = arena.Factory{ID: r.Target.ID, New: r.Target.New}
			factories[side.Other()] = arena.Factory{ID: opponent.ID, New: opponent.New}
			result, err := arena.Play(context.Background(), arena.MatchConfig{Seed: seed, ID: uint64(slot), StreamID: 0, MaxTurns: r.Config.MaxTurns}, factories)
			errs[i] = err
			if err == nil {
				scores[i] = training.Score{Index: slot, Seed: seed, Side: side, OpponentID: opponent.ID, Status: result.Record.Status, Outcome: result.Record.Outcome, Turns: result.Decisions, ForwardEvaluations: result.ForwardEvaluations}
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
func (r record) ActiveScores(final bool) []training.Score {
	if final {
		return r.FinalScores
	}
	return r.Active.Scores
}
func reportPath(id, sha string) string {
	return filepath.Join("experiments", id, "reports", sha+".json")
}
func (m *Manager) Report(id string) (json.RawMessage, error) {
	m.mu.Lock()
	r, ok := m.records[id]
	m.mu.Unlock()
	if !ok {
		return nil, ErrNotFound
	}
	if r.State != jobs.Completed || r.ReportSHA256 == "" {
		return nil, fmt.Errorf("%w: report is available only after completion", jobs.ErrConflict)
	}
	data, err := m.store.Read(reportPath(id, r.ReportSHA256), replay.MaxBytes)
	if err != nil {
		return nil, err
	}
	if hash(data) != r.ReportSHA256 {
		return nil, fmt.Errorf("report checksum mismatch")
	}
	return json.RawMessage(data), nil
}
