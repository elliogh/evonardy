// Package encoder defines the versioned public position input for neural models.
package encoder

import (
	"fmt"

	"evonardy/internal/game"
)

const Version = "long-nardy-encoder-v1"
const Size = 56
const ValuePerspective = "white"

// Vector uses fixed global points for both colors; it is never rotated by turn.
type Vector [Size]float64

// Encode accepts a full-turn position before the next roll, including terminal
// positions. Dice and private random state are not part of its input. Position
// validation cannot distinguish a draft board from a full-turn boundary; callers
// must supply the authoritative boundary position, not a draft preview.
func Encode(p game.Position) (Vector, error) {
	if err := game.ValidatePosition(p); err != nil {
		return Vector{}, fmt.Errorf("encode position: %w", err)
	}
	var vector Vector
	for player := game.White; player <= game.Black; player++ {
		for point, count := range p.Checkers[player] {
			vector[int(player)*24+point] = float64(count) / 15
		}
		vector[48+int(player)] = float64(p.BorneOff[player]) / 15
		if !p.FirstDone[player] {
			vector[52+int(player)] = 1
		}
	}
	vector[50+int(p.Turn)] = 1
	vector[54+int(p.Starter)] = 1
	return vector, nil
}
