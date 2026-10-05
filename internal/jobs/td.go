package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"time"

	"evonardy/internal/agent"
	"evonardy/internal/game"
	"evonardy/internal/library"
	"evonardy/internal/replay"
	"evonardy/internal/training"
)

// The durable history is bounded to fit the shared 16 MiB checkpoint envelope.
const maxTDJobGames = 10000

type NeuralCandidate struct {
	ID   string `json:"id"`
	Game int    `json:"game"`
}

type tdArchive struct {
	Version        int                    `json:"version"`
	Algorithm      string                 `json:"algorithm"`
	Ruleset        string                 `json:"ruleset"`
	EncoderVersion string                 `json:"encoder_version"`
	NetworkVersion string                 `json:"network_version"`
	Candidate      NeuralCandidate        `json:"candidate"`
	Parameters     agent.NeuralParameters `json:"parameters"`
	Metric         training.TDMetric      `json:"metric"`
}

func tdCandidate(n int) NeuralCandidate {
	return NeuralCandidate{ID: fmt.Sprintf("td-game-%06d", n), Game: n}
}

func syncTDSnapshot(r *record) {
	s := r.TDTraining
	c := s.Config
	r.Algorithm, r.TDConfig = s.Algorithm, &c
	r.Generation, r.GenerationGames, r.GenerationBudget = s.Games, s.Games, c.Games
	r.Counters, r.TDHistory = s.Counters, s.History
	if s.Games > 0 {
		candidate := tdCandidate(s.Games)
		r.NeuralCandidate = &candidate
	}
}

func (m *Manager) runTD(id string) {
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
		state, played, err := training.TrainTDGame(context.Background(), *r.TDTraining)
		m.mu.Lock()
		current := m.records[id]
		current.Revision++
		if err == nil {
			archive := tdArchive{Version: 1, Algorithm: state.Algorithm, Ruleset: state.Ruleset, EncoderVersion: state.EncoderVersion, NetworkVersion: state.NetworkVersion, Candidate: tdCandidate(state.Games), Parameters: state.Parameters, Metric: state.History[state.Games-1]}
			var data []byte
			data, err = json.Marshal(archive)
			if err == nil {
				err = m.store.WriteNew(generationPath(id, state.Games), data)
				if os.IsExist(err) {
					var old []byte
					old, err = m.store.Read(generationPath(id, state.Games), replay.MaxBytes)
					if err == nil && !bytes.Equal(data, old) {
						err = fmt.Errorf("existing neural checkpoint differs")
					}
				}
			}
			if err == nil {
				current.TDTraining, current.WatchReplay = &state, &played
				current.GenerationHashes = append(append([]string{}, current.GenerationHashes...), digest(data))
				if state.Games == state.Config.Games {
					current.State, current.Error = Completed, ""
				}
			}
		}
		if err != nil {
			current.State, current.Error = Failed, err.Error()
		}
		current.WallSeconds += time.Since(start).Seconds()
		if err := m.persist(current); err != nil {
			m.storageFailure(id, err)
			m.mu.Unlock()
			return
		}
		finished := terminal(current.State)
		m.mu.Unlock()
		if finished {
			return
		}
	}
}

// saveTD runs under the manager lock, just like the existing linear save path.
func (m *Manager) saveTD(ctx context.Context, r, next record, req SaveRequest) (library.Card, error) {
	if req.Generation < 1 || req.Generation > len(r.GenerationHashes) || req.CandidateID != tdCandidate(req.Generation).ID {
		return library.Card{}, fmt.Errorf("%w: choose a completed neural game checkpoint", ErrInvalid)
	}
	data, err := m.store.Read(generationPath(r.ID, req.Generation), replay.MaxBytes)
	if err != nil {
		return library.Card{}, err
	}
	if digest(data) != r.GenerationHashes[req.Generation-1] {
		return library.Card{}, fmt.Errorf("neural checkpoint checksum mismatch")
	}
	var archive tdArchive
	if err := library.DecodeJSON(data, &archive); err != nil {
		return library.Card{}, err
	}
	s := r.TDTraining
	if archive.Version != 1 || archive.Algorithm != s.Algorithm || archive.Ruleset != s.Ruleset || archive.EncoderVersion != s.EncoderVersion || archive.NetworkVersion != s.NetworkVersion || archive.Candidate != tdCandidate(req.Generation) || !reflect.DeepEqual(archive.Metric, s.History[req.Generation-1]) {
		return library.Card{}, fmt.Errorf("incompatible neural checkpoint")
	}
	if err := ctx.Err(); err != nil {
		return library.Card{}, err
	}
	card, err := m.bots.SaveNeural(req.Name, archive.Parameters[:], fmt.Sprintf("%s run %s, game %d", s.Algorithm, r.ID, req.Generation), r.ID+"/"+req.CandidateID)
	if err != nil {
		return library.Card{}, err
	}
	entry := next.Commands[req.CommandID]
	entry.BotID = card.ID
	next.Commands[req.CommandID] = entry
	next.Saved = append(append([]Publication{}, r.Saved...), Publication{BotID: card.ID, Generation: req.Generation, CandidateID: req.CandidateID})
	if err := m.persist(next); err != nil {
		return library.Card{}, err
	}
	return card, nil
}

func validateTDRecord(r record) error {
	s := r.TDTraining
	if r.Training != nil || r.FrozenEvaluation != nil || len(r.GenerationHashes) != s.Games || s.Config.Games > maxTDJobGames {
		return fmt.Errorf("invalid neural training archive references")
	}
	if err := training.ValidateTD(*s); err != nil {
		return err
	}
	if r.State == Completed && s.Games != s.Config.Games {
		return fmt.Errorf("incomplete neural training marked completed")
	}
	if r.Algorithm != s.Algorithm || r.TDConfig == nil || *r.TDConfig != s.Config || r.Generation != s.Games || r.GenerationGames != s.Games || r.GenerationBudget != s.Config.Games || r.Counters != s.Counters || len(r.TDHistory) != len(s.History) || (s.Games > 0 && !reflect.DeepEqual(r.TDHistory, s.History)) {
		return fmt.Errorf("neural public snapshot differs from checkpoint")
	}
	if (s.Games == 0 && r.NeuralCandidate != nil) || (s.Games > 0 && (r.NeuralCandidate == nil || *r.NeuralCandidate != tdCandidate(s.Games))) {
		return fmt.Errorf("neural candidate differs from checkpoint")
	}
	if r.WatchReplay == nil {
		if r.WatchedGame != nil || s.Games != 0 {
			return fmt.Errorf("missing neural watched game")
		}
		return nil
	}
	g := r.WatchReplay
	if s.Games < 1 || g.Generation != s.Games || g.Index != s.Games-1 || g.CandidateSide != game.Player((s.Games-1)%2) || g.CandidateID != s.Algorithm+"/self-play" || g.OpponentID != g.CandidateID || g.Replay.Bots != [2]string{g.CandidateID, g.OpponentID} || g.Replay.GameID != uint64(s.Games-1) || g.Replay.Seed != s.Config.Seed || g.Replay.MaxTurns != s.Config.MaxTurns || r.WatchedGame == nil || *r.WatchedGame != *gameNotice(g) {
		return fmt.Errorf("neural watched game identity mismatch")
	}
	metric := s.History[s.Games-1]
	if len(g.Replay.Events) != metric.Decisions || g.Replay.Status != metric.Status || !reflect.DeepEqual(g.Replay.Outcome, metric.Outcome) {
		return fmt.Errorf("neural watched game differs from training history")
	}
	_, err := replay.Play(g.Replay)
	return err
}
