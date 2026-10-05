package training_test

import (
	"context"
	"reflect"
	"testing"

	"evonardy/internal/replay"
	"evonardy/internal/training"
)

func TestVisibleTrainingMatchIsTheActualEvaluatedGame(t *testing.T) {
	s, err := training.New(small())
	if err != nil {
		t.Fatal(err)
	}
	tasks := training.NextTasks(s)
	wave, err := training.PlayWave(context.Background(), s, tasks)
	if err != nil {
		t.Fatal(err)
	}
	scores, err := training.Play(context.Background(), s, tasks)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wave.Scores, scores) {
		t.Fatal("watching changed evaluated games")
	}
	sample := wave.Sample
	if sample == nil || sample.Generation != 1 {
		t.Fatal("missing actual match")
	}
	score := wave.Scores[sample.Index-tasks[0].Index]
	if sample.Replay.GameID != uint64(score.Index) || sample.Replay.Seed != score.Seed || sample.Replay.Status != score.Status || *sample.Replay.Outcome != *score.Outcome || len(sample.Replay.Events) != score.Turns || sample.CandidateSide != score.Side || sample.Replay.Bots[score.Side] != sample.CandidateID || sample.Replay.Bots[score.Side.Other()] != score.OpponentID {
		t.Fatal("viewer replay differs from the training score")
	}
	if _, err := replay.Play(sample.Replay); err != nil {
		t.Fatal(err)
	}
	a, err := training.Add(s, wave.Scores)
	if err != nil {
		t.Fatal(err)
	}
	b, err := training.Add(s, scores)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatal("replay altered training state")
	}
	// Successive worker batches alternate samples across the two colors.
	next, err := training.PlayWave(context.Background(), a, training.NextTasks(a))
	if err != nil {
		t.Fatal(err)
	}
	if next.Sample.CandidateSide == sample.CandidateSide {
		t.Fatal("samples never change color")
	}
}
