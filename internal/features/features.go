// Package features defines the versioned white-perspective linear baseline.
package features

import "evonardy/internal/game"

const Version = "long-nardy-features-v1"
const Size = 9

type Vector [Size]float64

var Names = [Size]string{"pip_advantage", "head_advantage", "home_advantage", "off_advantage", "occupied_advantage", "block_advantage", "distribution_advantage", "rear_advantage", "mobility_advantage"}
var DefaultWeights = Vector{1, .45, .25, 2, .2, .25, .15, .2, .15}

// Encode uses cheap landing mobility, not a costly all-roll full-turn search.
func Encode(p game.Position) Vector {
	var raw [2]Vector
	for player := game.White; player <= game.Black; player++ {
		occupied, longest, run, rear, available := 0, 0, 0, 0, 0
		pip, squares, home := 0, 0, 0
		for point, count := range p.Checkers[player] {
			if count == 0 {
				continue
			}
			progress := game.Progress(player, point)
			pip += count * (24 - progress)
			squares += count * count
			if progress >= 18 {
				home += count
			}
			rear = max(rear, 24-progress)
			occupied++
			for die := 1; die <= 6; die++ {
				if progress+die < 24 && p.Checkers[player.Other()][game.PhysicalPoint(player, progress+die)] == 0 {
					available++
				}
			}
		}
		for progress := 0; progress < 24; progress++ {
			if p.Checkers[player][game.PhysicalPoint(player.Other(), progress)] > 0 {
				run++
				longest = max(longest, run)
			} else {
				run = 0
			}
		}
		raw[player] = Vector{-float64(pip) / 360, -float64(p.Checkers[player][game.Head(player)]) / 15, float64(home) / 15, float64(p.BorneOff[player]) / 15, float64(occupied) / 15, float64(longest) / 15, -float64(squares) / 225, -float64(rear) / 24, float64(available) / 90}
	}
	var vector Vector
	for i := range vector {
		vector[i] = raw[game.White][i] - raw[game.Black][i]
	}
	return vector
}

func Score(p game.Position, weights Vector) float64 {
	vector := Encode(p)
	score := 0.0
	for i, weight := range weights {
		score += weight * vector[i]
	}
	return score
}
