// Package game implements long-nardy-fnr2026-nocube-v1 without I/O or randomness.
package game

import "fmt"

const Ruleset = "long-nardy-fnr2026-nocube-v1"
const Off = 24

type Player int

const (
	White Player = iota
	Black
)

func (p Player) Other() Player { return 1 - p }
func (p Player) Valid() bool   { return p == White || p == Black }

func Head(p Player) int                        { return int(p) * 12 }
func Progress(p Player, point int) int         { return (point - Head(p) + 24) % 24 }
func PhysicalPoint(p Player, progress int) int { return (Head(p) + progress) % 24 }

// Position is a value snapshot at a full-turn boundary. Arrays make copying safe.
type Position struct {
	Ruleset   string     `json:"ruleset"`
	Checkers  [2][24]int `json:"checkers"`
	BorneOff  [2]int     `json:"borne_off"`
	Turn      Player     `json:"turn"`
	Starter   Player     `json:"starter"`
	FirstDone [2]bool    `json:"first_done"`
}

func Initial(starter Player) Position {
	p := Position{Ruleset: Ruleset, Turn: starter, Starter: starter}
	p.Checkers[White][0], p.Checkers[Black][12] = 15, 15
	return p
}

func ValidatePosition(p Position) error {
	if p.Ruleset != Ruleset {
		return fmt.Errorf("unsupported ruleset %q", p.Ruleset)
	}
	if !p.Turn.Valid() || !p.Starter.Valid() {
		return fmt.Errorf("invalid player")
	}
	for player := White; player <= Black; player++ {
		total := p.BorneOff[player]
		if total < 0 || total > 15 {
			return fmt.Errorf("invalid borne-off count")
		}
		for point, count := range p.Checkers[player] {
			if count < 0 || count > 15 {
				return fmt.Errorf("invalid checker count at %d", point)
			}
			if count > 0 && p.Checkers[player.Other()][point] > 0 {
				return fmt.Errorf("overlapping colors at %d", point)
			}
			total += count
		}
		if total != 15 {
			return fmt.Errorf("player %d has %d checkers, want 15", player, total)
		}
		if forbiddenBlock(p, player) {
			return fmt.Errorf("player %d imprisons all opposing checkers", player)
		}
	}
	if p.BorneOff[0] == 15 && p.BorneOff[1] == 15 {
		return fmt.Errorf("two winners")
	}
	return nil
}

// A block is measured on the opponent's finite route, never across its endpoints.
func forbiddenBlock(p Position, blocker Player) bool {
	opponent := blocker.Other()
	if p.BorneOff[opponent] > 0 {
		return false
	}
	farthest := -1
	for point, count := range p.Checkers[opponent] {
		if count > 0 {
			farthest = max(farthest, Progress(opponent, point))
		}
	}
	run := 0
	for progress := 0; progress < 24; progress++ {
		if p.Checkers[blocker][PhysicalPoint(opponent, progress)] > 0 {
			run++
			if run >= 6 && progress-run+1 > farthest {
				return true
			}
		} else {
			run = 0
		}
	}
	return false
}

type Outcome struct {
	Winner Player `json:"winner"`
	Points int    `json:"points"`
	Mars   bool   `json:"mars"`
}

func Result(p Position) (Outcome, bool) {
	for player := White; player <= Black; player++ {
		if p.BorneOff[player] == 15 {
			mars := p.BorneOff[player.Other()] == 0
			points := 1
			if mars {
				points = 2
			}
			return Outcome{Winner: player, Points: points, Mars: mars}, true
		}
	}
	return Outcome{}, false
}
