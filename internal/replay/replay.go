// Package replay records actual actions and verifies them without invoking agents.
package replay

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"evonardy/internal/game"
	"fmt"
	"io"
)

const Version = 1
const Completed = "completed"
const Truncated = "truncated"
const MaxBytes = 16 << 20
const MaxEvents = 10000

type Event struct {
	Dice      game.Dice `json:"dice"`
	Turn      game.Turn `json:"turn"`
	AfterHash string    `json:"after_hash"`
}

type Record struct {
	Version     int           `json:"version"`
	Ruleset     string        `json:"ruleset"`
	Seed        uint64        `json:"seed"`
	GameID      uint64        `json:"game_id"`
	Bots        [2]string     `json:"bots"`
	Initial     game.Position `json:"initial"`
	InitialHash string        `json:"initial_hash"`
	Opening     []game.Dice   `json:"opening"`
	Events      []Event       `json:"events"`
	MaxTurns    int           `json:"max_turns"`
	Status      string        `json:"status"`
	Outcome     *game.Outcome `json:"outcome"`
	FinalHash   string        `json:"final_hash"`
}

func Hash(p game.Position) string {
	data, _ := json.Marshal(p)
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func New(p game.Position, opening []game.Dice, bots [2]string, seed, id uint64, limit int) Record {
	return Record{Version: Version, Ruleset: game.Ruleset, Seed: seed, GameID: id, Bots: bots, Initial: p, InitialHash: Hash(p), Opening: append([]game.Dice{}, opening...), Events: []Event{}, MaxTurns: limit}
}

func (r *Record) Append(dice game.Dice, turn game.Turn, after game.Position) {
	copyTurn := game.Turn{Steps: append([]game.Step{}, turn.Steps...)}
	r.Events = append(r.Events, Event{Dice: dice, Turn: copyTurn, AfterHash: Hash(after)})
}

func (r *Record) Finish(final game.Position, status string) {
	r.Status = status
	r.FinalHash = Hash(final)
	r.Outcome = nil
	if result, terminal := game.Result(final); terminal {
		r.Outcome = &result
	}
}

// Positions validates and reconstructs an unfinished or finished history.
// The first item is the initial position; every later item follows a full turn.
func Positions(r Record) ([]game.Position, error) {
	p := r.Initial
	positions := []game.Position{p}
	if r.Version != Version || r.Ruleset != game.Ruleset {
		return nil, fmt.Errorf("unsupported replay version or ruleset")
	}
	if err := game.ValidatePosition(p); err != nil {
		return nil, err
	}
	if r.InitialHash != Hash(p) {
		return nil, fmt.Errorf("initial position hash mismatch")
	}
	if r.MaxTurns < 1 || r.MaxTurns > MaxEvents || len(r.Events) > r.MaxTurns {
		return nil, fmt.Errorf("invalid replay turn limit")
	}
	if len(r.Opening) > MaxEvents {
		return nil, fmt.Errorf("too many opening attempts")
	}
	if len(r.Opening) > 0 {
		for i, dice := range r.Opening {
			if err := dice.Validate(); err != nil {
				return nil, err
			}
			last := i == len(r.Opening)-1
			if (dice[0] != dice[1]) != last {
				return nil, fmt.Errorf("invalid opening tie sequence")
			}
		}
		last := r.Opening[len(r.Opening)-1]
		starter := game.White
		if last[1] > last[0] {
			starter = game.Black
		}
		if p != game.Initial(starter) || (len(r.Events) > 0 && r.Events[0].Dice != last) {
			return nil, fmt.Errorf("opening position/dice mismatch")
		}
	} else if !p.FirstDone[0] || !p.FirstDone[1] {
		return nil, fmt.Errorf("opening history missing")
	}
	for i, event := range r.Events {
		next, err := game.ApplyTurn(p, event.Dice, event.Turn)
		if err != nil {
			return nil, fmt.Errorf("turn %d: %w", i, err)
		}
		if Hash(next) != event.AfterHash {
			return nil, fmt.Errorf("turn %d: state hash mismatch", i)
		}
		p = next
		positions = append(positions, p)
	}
	return positions, nil
}

func Play(r Record) (game.Position, error) {
	positions, err := Positions(r)
	if err != nil {
		return r.Initial, err
	}
	p := positions[len(positions)-1]

	if Hash(p) != r.FinalHash {
		return p, fmt.Errorf("final state hash mismatch")
	}
	result, terminal := game.Result(p)
	switch r.Status {
	case Completed:
		if !terminal || r.Outcome == nil || *r.Outcome != result {
			return p, fmt.Errorf("completed replay result mismatch")
		}
	case Truncated:
		if terminal || r.Outcome != nil || len(r.Events) != r.MaxTurns {
			return p, fmt.Errorf("truncated replay is not at its nonterminal limit")
		}
	default:
		return p, fmt.Errorf("invalid replay status %q", r.Status)
	}
	return p, nil
}

func Encode(w io.Writer, r Record) error {
	if _, err := Play(r); err != nil {
		return err
	}
	e := json.NewEncoder(w)
	e.SetIndent("", "  ")
	return e.Encode(r)
}

func Decode(reader io.Reader) (Record, error) {
	var r Record
	data, err := io.ReadAll(io.LimitReader(reader, MaxBytes+1))
	if err != nil {
		return r, err
	}
	if len(data) > MaxBytes {
		return r, fmt.Errorf("replay exceeds %d bytes", MaxBytes)
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&r); err != nil {
		return r, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return r, fmt.Errorf("trailing replay data")
	}
	_, err = Play(r)
	return r, err
}
