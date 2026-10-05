package features_test

import (
	"evonardy/internal/features"
	"evonardy/internal/game"
	"math"
	"testing"
)

func TestFeaturesWhitePerspectiveAndNormalization(t *testing.T) {
	p := game.Initial(game.White)
	if v := features.Encode(p); v != (features.Vector{}) {
		t.Fatalf("initial symmetry: %v", v)
	}
	p.Checkers[0][0] = 14
	p.Checkers[0][6] = 1
	v := features.Encode(p)
	if math.Abs(v[0]-6.0/360) > 1e-12 || math.Abs(v[1]-1.0/15) > 1e-12 || math.Abs(v[4]-1.0/15) > 1e-12 {
		t.Fatalf("worked example: %v", v)
	}
	q := p
	for side := game.White; side <= game.Black; side++ {
		for point, count := range p.Checkers[side] {
			q.Checkers[side.Other()][(point+12)%24] = count
		}
	}
	q.BorneOff = [2]int{p.BorneOff[1], p.BorneOff[0]}
	w := features.Encode(q)
	for i := range v {
		if math.Abs(v[i]+w[i]) > 1e-12 || math.Abs(v[i]) > 1 {
			t.Fatalf("feature %s: %f %f", features.Names[i], v[i], w[i])
		}
	}
}

func BenchmarkHeuristicScore(b *testing.B) {
	p := game.Initial(game.White)
	b.ReportAllocs()
	for b.Loop() {
		features.Score(p, features.DefaultWeights)
	}
}
