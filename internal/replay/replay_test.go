package replay_test

import (
	"bytes"
	"evonardy/internal/game"
	"evonardy/internal/replay"
	"testing"
)

func finishedReplay(t *testing.T) replay.Record {
	t.Helper()
	p := game.Initial(game.White)
	p.Checkers[0] = [24]int{}
	p.Checkers[0][23] = 1
	p.BorneOff[0] = 14
	p.FirstDone = [2]bool{true, true}
	dice := game.Dice{6, 6}
	turn := game.Turn{Steps: []game.Step{{From: 23, To: 24, Die: 6}}}
	q, err := game.ApplyTurn(p, dice, turn)
	if err != nil {
		t.Fatal(err)
	}
	r := replay.New(p, nil, [2]string{"heuristic", "random"}, 7, 0, 100)
	r.Append(dice, turn, q)
	r.Finish(q, replay.Completed)
	return r
}

func TestReplayRoundTripWithoutAgents(t *testing.T) {
	r := finishedReplay(t)
	var data bytes.Buffer
	if err := replay.Encode(&data, r); err != nil {
		t.Fatal(err)
	}
	loaded, err := replay.Decode(&data)
	if err != nil {
		t.Fatal(err)
	}
	final, err := replay.Play(loaded)
	if err != nil {
		t.Fatal(err)
	}
	if final.BorneOff[0] != 15 || loaded.Outcome == nil || !loaded.Outcome.Mars {
		t.Fatal("wrong restored result")
	}
}

func TestReplayRejectsCorruptionAndIncompatibleRules(t *testing.T) {
	for _, change := range []func(*replay.Record){
		func(r *replay.Record) { r.Events[0].AfterHash = "bad" },
		func(r *replay.Record) { r.FinalHash = "bad" },
		func(r *replay.Record) { r.Events[0].Turn.Steps[0].To = 0 },
		func(r *replay.Record) { r.Version = 99 },
		func(r *replay.Record) { r.Ruleset = "other" },
		func(r *replay.Record) { r.Status = replay.Truncated },
		func(r *replay.Record) { r.Outcome.Points = 1 },
	} {
		r := finishedReplay(t)
		change(&r)
		if _, err := replay.Play(r); err == nil {
			t.Fatal("corrupt replay accepted")
		}
	}
	var raw bytes.Buffer
	if err := replay.Encode(&raw, finishedReplay(t)); err != nil {
		t.Fatal(err)
	}
	raw.WriteString("{}")
	if _, err := replay.Decode(&raw); err == nil {
		t.Fatal("trailing JSON accepted")
	}
}
