package jobs

import (
	"encoding/json"
	"fmt"

	"evonardy/internal/game"
	"evonardy/internal/replay"
	"evonardy/internal/training"
)

// GameNotice stays small enough for ordinary job snapshots and list routes.
type GameNotice struct {
	Key           string      `json:"key"`
	Generation    int         `json:"generation"`
	Index         int         `json:"index"`
	CandidateID   string      `json:"candidate_id"`
	OpponentID    string      `json:"opponent_id"`
	CandidateSide game.Player `json:"candidate_side"`
	Turns         int         `json:"turns"`
	Status        string      `json:"status"`
}
type WatchedGame struct {
	GameNotice
	Replay    replay.Record   `json:"replay"`
	Positions []game.Position `json:"positions"`
}

func gameNotice(g *training.PlayedGame) *GameNotice {
	if g == nil {
		return nil
	}
	return &GameNotice{Key: g.Key(), Generation: g.Generation, Index: g.Index, CandidateID: g.CandidateID, OpponentID: g.OpponentID, CandidateSide: g.CandidateSide, Turns: len(g.Replay.Events), Status: g.Replay.Status}
}

// Watch reconstructs Go-owned positions from the latest actual training replay.
// It never runs inference or mutates the job, and releases the scheduler lock first.
func (m *Manager) Watch(id string) (WatchedGame, error) {
	m.mu.Lock()
	r, ok := m.records[id]
	m.mu.Unlock()
	if !ok || r.Kind != Training || r.WatchReplay == nil {
		return WatchedGame{}, ErrNotFound
	}
	data, err := json.Marshal(r.WatchReplay)
	if err != nil {
		return WatchedGame{}, err
	}
	var sample training.PlayedGame
	if err := json.Unmarshal(data, &sample); err != nil {
		return WatchedGame{}, err
	}
	positions, err := replay.Positions(sample.Replay)
	if err != nil {
		return WatchedGame{}, err
	}
	return WatchedGame{GameNotice: *gameNotice(&sample), Replay: sample.Replay, Positions: positions}, nil
}
func validateWatch(r record) error {
	g := r.WatchReplay
	if g == nil {
		if r.WatchedGame != nil {
			return fmt.Errorf("missing watched replay")
		}
		return nil
	}
	c := r.Training.Config
	if g.Generation < 1 || g.Generation > c.Generations || g.Generation < r.Training.Generation || g.Generation > r.Training.Generation+1 || g.Index < 0 || g.Index >= training.Slots(*r.Training) || !g.CandidateSide.Valid() {
		return fmt.Errorf("invalid watched match identity")
	}
	perCandidate := training.GamesPerCandidate(*r.Training)
	opponent := (g.Index % perCandidate) / (c.PairsPerOpponent * 2)
	pair := (g.Index / 2) % c.PairsPerOpponent
	if g.CandidateSide != game.Player(g.Index%2) || g.CandidateID != fmt.Sprintf("g%04d-c%04d", g.Generation-1, g.Index/perCandidate) || g.OpponentID != training.SelectionOpponent(*r.Training, opponent).ID || g.Replay.Bots[g.CandidateSide] != g.CandidateID || g.Replay.Bots[g.CandidateSide.Other()] != g.OpponentID || g.Replay.GameID != uint64(g.Index) || g.Replay.Seed != training.SelectionPairSeed(*r.Training, g.Generation-1, opponent, pair) || g.Replay.MaxTurns != c.MaxTurns {
		return fmt.Errorf("watched match schedule mismatch")
	}
	if r.WatchedGame == nil || *r.WatchedGame != *gameNotice(g) {
		return fmt.Errorf("watched match notice mismatch")
	}
	if _, err := replay.Play(g.Replay); err != nil {
		return fmt.Errorf("watched replay: %w", err)
	}
	return nil
}
