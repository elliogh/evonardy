package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"slices"
	"time"

	"evonardy/internal/game"
	"evonardy/internal/library"
	"evonardy/internal/random"
	"evonardy/internal/replay"
	"evonardy/internal/training"
)

type ExecutionInfo struct {
	Platform       string `json:"platform"`
	GoVersion      string `json:"go_version"`
	Workers        int    `json:"workers"`
	Ruleset        string `json:"ruleset"`
	EncoderVersion string `json:"encoder_version"`
	NetworkVersion string `json:"network_version"`
	RandomContract string `json:"random_contract"`
}

func neuroExecution(s training.NeuroState) *ExecutionInfo {
	return &ExecutionInfo{Platform: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(), Workers: s.Config.Workers, Ruleset: s.Ruleset, EncoderVersion: s.EncoderVersion, NetworkVersion: s.NetworkVersion, RandomContract: s.RandomContract}
}

type PopulationProgress struct {
	Phase          string `json:"phase"`
	TrainingDone   []int  `json:"training_done,omitempty"`
	TrainingGames  uint64 `json:"training_games"`
	SelectionGames uint64 `json:"selection_games"`
	TotalBudget    int    `json:"total_budget"`
}

func syncNeuroSnapshot(r *record) {
	s := r.NeuroTraining
	c := s.Config
	r.Algorithm = s.Algorithm
	r.Generation = s.Generation
	r.Counters = s.Counters
	r.History = s.History
	r.GenerationGames = len(s.Results)
	r.GenerationBudget = training.NeuroSlots(*s)
	p := PopulationProgress{Phase: "selection", SelectionGames: s.Counters.Games, TotalBudget: c.Generations * training.NeuroSlots(*s)}
	if s.Hybrid != nil {
		h := s.Hybrid
		r.HybridConfig = &h.Config
		r.Config = nil
		p.TrainingDone = slices.Clone(h.TrainingDone)
		p.TrainingGames = h.RoundCounters.Games
		for _, work := range h.History {
			p.TrainingGames += work.Games
		}
		p.SelectionGames -= p.TrainingGames
		p.TotalBudget += c.Generations * training.HybridParticipants * h.Config.GamesPerRound
		if !training.HybridTrainingReady(*s) {
			p.Phase = "training"
		}
	} else {
		r.Config = &c
	}
	if s.Generation == c.Generations {
		p.Phase = "completed"
		if s.Hybrid != nil {
			for i := range p.TrainingDone {
				p.TrainingDone[i] = s.Hybrid.Config.GamesPerRound
			}
		}
	}
	r.PopulationProgress = &p
}
func (m *Manager) runNeuro(id string) {
	for {
		m.mu.Lock()
		r := m.records[id]
		if m.closing || r.State == Stopping {
			r.State, r.Error = Stopped, "Stopped by user; checkpoint ready"
			if m.closing {
				r.State, r.Error = Interrupted, "Server stopped; checkpoint ready"
			}
			r.Revision++
			if err := m.persist(r); err != nil {
				m.storageFailure(id, err)
			}
			m.mu.Unlock()
			return
		}
		m.mu.Unlock()
		start := time.Now()
		state, gen, played, err := training.StepNeuro(context.Background(), *r.NeuroTraining)
		m.mu.Lock()
		current := m.records[id]
		current.Revision++
		// Completed learner games and failed selection waves are actual work. A failed
		// archive write retains the previous committed boundary instead.
		if gen != nil && err == nil {
			var data []byte
			data, err = json.Marshal(gen)
			if err == nil {
				err = m.store.WriteNew(generationPath(id, gen.Number), data)
				if os.IsExist(err) {
					var old []byte
					old, err = m.store.Read(generationPath(id, gen.Number), replay.MaxBytes)
					if err == nil && !bytes.Equal(data, old) {
						err = fmt.Errorf("existing neural generation differs")
					}
				}
			}
			if err == nil {
				current.GenerationHashes = append(slices.Clone(current.GenerationHashes), digest(data))
				current.NeuralCandidates = training.NeuroSummaries(gen.Ranked)
				current.Replacements = slices.Clone(gen.Replacements)
			}
		}
		if gen == nil || err == nil {
			current.NeuroTraining = &state
			if played != nil {
				current.WatchReplay = played
			}
		}
		if err != nil {
			current.State, current.Error = Failed, err.Error()
		} else if state.Generation == state.Config.Generations {
			current.State, current.Error = Completed, ""
		}
		current.WallSeconds += time.Since(start).Seconds()
		if err := m.persist(current); err != nil {
			m.storageFailure(id, err)
			m.mu.Unlock()
			return
		}
		done := terminal(current.State)
		m.mu.Unlock()
		if done {
			return
		}
	}
}
func (m *Manager) neuroGeneration(r record, n int) (training.NeuroGeneration, error) {
	var gen training.NeuroGeneration
	data, err := m.store.Read(generationPath(r.ID, n), replay.MaxBytes)
	if err != nil {
		return gen, err
	}
	if digest(data) != r.GenerationHashes[n-1] {
		return gen, fmt.Errorf("neural generation checksum mismatch")
	}
	if err := library.DecodeJSON(data, &gen); err != nil {
		return gen, err
	}
	s := r.NeuroTraining
	if gen.Number != n || gen.Algorithm != s.Algorithm || gen.Ruleset != s.Ruleset || gen.EncoderVersion != s.EncoderVersion || gen.NetworkVersion != s.NetworkVersion || !reflect.DeepEqual(gen.Metric, s.History[n-1]) || len(gen.Ranked) != s.Config.Population {
		return gen, fmt.Errorf("incompatible neural generation")
	}
	return gen, nil
}
func (m *Manager) neuroGenerationView(r record, n int) (training.Generation, error) {
	gen, err := m.neuroGeneration(r, n)
	if err != nil {
		return training.Generation{}, err
	}
	return training.Generation{Algorithm: gen.Algorithm, Ruleset: gen.Ruleset, FeaturesVersion: gen.EncoderVersion, Number: n, Ranked: []training.Candidate{}, NeuralRanked: training.NeuroSummaries(gen.Ranked), Scores: gen.Scores, Metric: gen.Metric, Replacements: gen.Replacements, TrainingCounters: gen.TrainingCounters}, nil
}
func (m *Manager) saveNeuro(ctx context.Context, r, next record, req SaveRequest) (library.Card, error) {
	if req.Generation < 1 || req.Generation > len(r.GenerationHashes) {
		return library.Card{}, fmt.Errorf("%w: choose an evaluated neural generation", ErrInvalid)
	}
	gen, err := m.neuroGeneration(r, req.Generation)
	if err != nil {
		return library.Card{}, err
	}
	for _, p := range gen.Ranked {
		if p.ID != req.CandidateID {
			continue
		}
		if err := ctx.Err(); err != nil {
			return library.Card{}, err
		}
		card, err := m.bots.SaveNeural(req.Name, p.Parameters[:], fmt.Sprintf("%s run %s, round %d", gen.Algorithm, r.ID, req.Generation), r.ID+"/"+p.ID)
		if err != nil {
			return card, err
		}
		entry := next.Commands[req.CommandID]
		entry.BotID = card.ID
		next.Commands[req.CommandID] = entry
		next.Saved = append(slices.Clone(r.Saved), Publication{BotID: card.ID, Generation: req.Generation, CandidateID: p.ID})
		if err := m.persist(next); err != nil {
			return library.Card{}, err
		}
		return card, nil
	}
	return library.Card{}, fmt.Errorf("%w: neural candidate not found", ErrInvalid)
}
func validateNeuroRecord(r record) error {
	s := r.NeuroTraining
	if r.Training != nil || r.TDTraining != nil || r.FrozenEvaluation != nil || len(r.GenerationHashes) != s.Generation {
		return fmt.Errorf("invalid neural population archive references")
	}
	if r.Execution != nil && (r.Execution.Platform == "" || r.Execution.GoVersion == "" || r.Execution.Workers != s.Config.Workers || r.Execution.Ruleset != s.Ruleset || r.Execution.EncoderVersion != s.EncoderVersion || r.Execution.NetworkVersion != s.NetworkVersion || r.Execution.RandomContract != s.RandomContract) {
		return fmt.Errorf("incompatible population execution metadata")
	}
	if err := training.ValidateNeuro(*s); err != nil {
		return err
	}
	if r.State == Completed && s.Generation != s.Config.Generations {
		return fmt.Errorf("incomplete neural population marked completed")
	}
	expected := r
	syncNeuroSnapshot(&expected)
	if r.Algorithm != expected.Algorithm || r.Generation != expected.Generation || r.GenerationGames != expected.GenerationGames || r.GenerationBudget != expected.GenerationBudget || r.Counters != expected.Counters || !reflect.DeepEqual(r.Config, expected.Config) || !reflect.DeepEqual(r.HybridConfig, expected.HybridConfig) || !reflect.DeepEqual(r.PopulationProgress, expected.PopulationProgress) || !reflect.DeepEqual(r.History, expected.History) {
		return fmt.Errorf("neural population snapshot differs from checkpoint")
	}
	if (s.Generation == 0 && len(r.NeuralCandidates) != 0) || (s.Generation > 0 && (len(r.NeuralCandidates) != s.Config.Population || r.NeuralCandidates[0].ID != s.History[s.Generation-1].BestCandidateID)) {
		return fmt.Errorf("invalid neural candidate summaries")
	}
	return validateNeuroWatch(r)
}

func validateNeuroWatch(r record) error {
	s := r.NeuroTraining
	g := r.WatchReplay
	if g == nil {
		if r.WatchedGame != nil || s.Counters.Games != 0 {
			return fmt.Errorf("missing neural population replay")
		}
		return nil
	}
	if r.WatchedGame == nil || *r.WatchedGame != *gameNotice(g) || g.Generation < 1 || g.Generation > s.Generation+1 || g.Generation < max(1, s.Generation) || g.Generation > s.Config.Generations || g.Index < 0 || !g.CandidateSide.Valid() || g.Replay.MaxTurns != s.Config.MaxTurns {
		return fmt.Errorf("neural population watch identity differs")
	}
	offset := 0
	if s.Hybrid != nil {
		offset = training.HybridParticipants * s.Hybrid.Config.GamesPerRound
	}
	if g.Index < offset {
		slot := g.Index / s.Hybrid.Config.GamesPerRound
		count := g.Index % s.Hybrid.Config.GamesPerRound
		id := fmt.Sprintf("n%04d-c%04d", g.Generation-1, slot)
		seed := random.New(s.Config.Seed, fmt.Sprintf("hybrid/round/%d/participant/%d", g.Generation-1, slot), uint64(count)).Uint64()
		if g.CandidateID != id || g.OpponentID != id || g.Replay.Bots != [2]string{id, id} || g.Replay.Seed != seed || g.Replay.GameID != 0 || g.CandidateSide != game.White {
			return fmt.Errorf("Hybrid watched training schedule differs")
		}
	} else {
		index := g.Index - offset
		perCandidate := s.Config.PairsPerOpponent * 4
		if index >= training.NeuroSlots(*s) {
			return fmt.Errorf("neural watched selection index differs")
		}
		slot := index / perCandidate
		opponent := (index % perCandidate) / (s.Config.PairsPerOpponent * 2)
		pair := (index / 2) % s.Config.PairsPerOpponent
		side := game.Player(index % 2)
		if g.CandidateID != fmt.Sprintf("n%04d-c%04d", g.Generation-1, slot) || g.CandidateSide != side || g.OpponentID != s.Development[opponent].ID || g.Replay.Bots[side] != g.CandidateID || g.Replay.Bots[side.Other()] != g.OpponentID || g.Replay.GameID != uint64(index) || g.Replay.Seed != training.PairSeed(s.Config.Seed, "ga/development", opponent, pair) {
			return fmt.Errorf("neural watched selection schedule differs")
		}
	}
	_, err := replay.Play(g.Replay)
	return err
}
