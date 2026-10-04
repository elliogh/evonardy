// Package app owns durable human sessions. The game engine owns all legality.
package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"evonardy/internal/game"
	"evonardy/internal/library"
	"evonardy/internal/random"
	"evonardy/internal/replay"
	"evonardy/internal/storage"
)

const AwaitingRoll = "awaiting_roll"
const Moving = "moving"
const Finished = "finished"

var ErrInvalid = errors.New("invalid game command")
var ErrNotFound = errors.New("game not found")
var ErrConflict = errors.New("command or version conflict")

type Command = library.Command
type CreateRequest struct {
	Command
	BotID string      `json:"bot_id"`
	Human game.Player `json:"human"`
}
type DraftRequest struct {
	Command
	Prefix []game.Step `json:"prefix"`
}
type TurnRequest struct {
	Command
	Turn game.Turn `json:"turn"`
}
type HistoryEvent struct {
	Actor game.Player `json:"actor"`
	replay.Event
}
type GameSummary struct {
	ID        string        `json:"id"`
	Version   uint64        `json:"version"`
	BotID     string        `json:"bot_id"`
	BotName   string        `json:"bot_name"`
	Human     game.Player   `json:"human"`
	Phase     string        `json:"phase"`
	CreatedAt string        `json:"created_at"`
	UpdatedAt string        `json:"updated_at"`
	Outcome   *game.Outcome `json:"outcome"`
}
type Snapshot struct {
	GameSummary
	Position      game.Position      `json:"position"`
	DraftPosition game.Position      `json:"draft_position"`
	Dice          *game.Dice         `json:"dice"`
	Draft         []game.Step        `json:"draft"`
	Continuations game.Continuations `json:"continuations"`
	RemainingDice []int              `json:"remaining_dice"`
	Opening       []game.Dice        `json:"opening"`
	History       []HistoryEvent     `json:"history"`
}
type session struct {
	FormatVersion int `json:"format_version"`
	GameSummary
	Position     game.Position              `json:"position"`
	Dice         *game.Dice                 `json:"dice"`
	Draft        []game.Step                `json:"draft"`
	Record       replay.Record              `json:"record"`
	Seed         uint64                     `json:"seed"`
	SideTurns    [2]uint64                  `json:"side_turns"`
	CreationHash string                     `json:"creation_hash"`
	Commands     map[string]library.Receipt `json:"commands"`
}
type Options struct{ Seed func() (uint64, error) }
type entry struct {
	mu    sync.Mutex
	state session
}
type Service struct {
	store    *storage.Store
	library  *library.Library
	seed     func() (uint64, error)
	mu       sync.Mutex
	createMu sync.Mutex
	entries  map[string]*entry
}

func New(store *storage.Store, bots *library.Library, options Options) *Service {
	seed := options.Seed
	if seed == nil {
		seed = func() (uint64, error) {
			var b [8]byte
			_, err := rand.Read(b[:])
			return binary.LittleEndian.Uint64(b[:]), err
		}
	}
	return &Service{store: store, library: bots, seed: seed, entries: map[string]*entry{}}
}
func fingerprint(op string, req any) string {
	data, _ := json.Marshal(req)
	sum := sha256.Sum256(append([]byte(op+":"), data...))
	return hex.EncodeToString(sum[:])
}
func path(id string) string { return filepath.Join("games", id+".json") }
func timestamp() string     { return time.Now().UTC().Format(time.RFC3339Nano) }
func (s *Service) load(id string) (*entry, error) {
	if !library.ValidCommandID(id) {
		return nil, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[id]; ok {
		return e, nil
	}
	data, err := s.store.Read(path(id), replay.MaxBytes)
	if os.IsNotExist(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var state session
	if err := library.DecodeJSON(data, &state); err != nil {
		return nil, fmt.Errorf("invalid saved session: %w", err)
	}
	if err := validate(state, id); err != nil {
		return nil, err
	}
	if len(s.entries) >= 1000 {
		return nil, fmt.Errorf("session capacity reached")
	}
	e := &entry{state: state}
	s.entries[id] = e
	return e, nil
}
func validate(st session, id string) error {
	if st.FormatVersion != 1 || st.ID != id || st.Version < 1 || !st.Human.Valid() || st.Record.Seed != st.Seed || len(st.Record.Opening) == 0 || len(st.Commands) > 20000 || st.Commands == nil {
		return fmt.Errorf("invalid saved session contract")
	}
	positions, err := replay.Positions(st.Record)
	if err != nil {
		return err
	}
	if positions[len(positions)-1] != st.Position {
		return fmt.Errorf("session/history mismatch")
	}
	counts := [2]uint64{}
	for i := range st.Record.Events {
		counts[positions[i].Turn]++
	}
	if st.Dice != nil {
		counts[st.Position.Turn]++
	}
	if counts != st.SideTurns {
		return fmt.Errorf("session dice counters mismatch")
	}
	switch st.Phase {
	case Moving:
		if st.Dice == nil || st.Position.Turn != st.Human {
			return fmt.Errorf("invalid moving session")
		}
		if _, _, err := game.PreviewTurn(st.Position, *st.Dice, st.Draft); err != nil {
			return err
		}
		if len(st.Record.Events) == 0 && *st.Dice != st.Record.Opening[len(st.Record.Opening)-1] {
			return fmt.Errorf("opening dice mismatch")
		}
	case AwaitingRoll:
		if st.Dice != nil || len(st.Draft) != 0 || st.Position.Turn != st.Human {
			return fmt.Errorf("invalid awaiting session")
		}
	case Finished:
		if st.Dice != nil || len(st.Draft) != 0 {
			return fmt.Errorf("invalid finished session")
		}
		if _, err := replay.Play(st.Record); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid session phase")
	}
	result, terminal := game.Result(st.Position)
	if terminal != (st.Phase == Finished) || (terminal && (st.Outcome == nil || *st.Outcome != result)) || (!terminal && (st.Outcome != nil || st.Record.Status != "")) {
		return fmt.Errorf("session outcome mismatch")
	}
	return nil
}
func snapshot(st session) (Snapshot, error) {
	x := Snapshot{GameSummary: st.GameSummary, Position: st.Position, DraftPosition: st.Position, Draft: append([]game.Step{}, st.Draft...), Opening: append([]game.Dice{}, st.Record.Opening...), History: []HistoryEvent{}, RemainingDice: []int{}, Continuations: game.Continuations{Next: []game.Step{}}}
	if st.Outcome != nil {
		o := *st.Outcome
		x.Outcome = &o
	}
	actor := st.Record.Initial.Turn
	for _, ev := range st.Record.Events {
		ev.Turn.Steps = append([]game.Step{}, ev.Turn.Steps...)
		x.History = append(x.History, HistoryEvent{Actor: actor, Event: ev})
		actor = 1 - actor
	}
	if st.Dice != nil {
		d := *st.Dice
		x.Dice = &d
		p, c, err := game.PreviewTurn(st.Position, d, st.Draft)
		if err != nil {
			return x, err
		}
		x.DraftPosition, x.Continuations = p, c
		if x.Continuations.Next == nil {
			x.Continuations.Next = []game.Step{}
		}
		x.RemainingDice = []int{d[0], d[1]}
		if d[0] == d[1] {
			x.RemainingDice = append(x.RemainingDice, d[0], d[0])
		}
		for _, step := range st.Draft {
			i := slices.Index(x.RemainingDice, step.Die)
			if i >= 0 {
				x.RemainingDice = slices.Delete(x.RemainingDice, i, i+1)
			}
		}
	}
	return x, nil
}
func (s *Service) Get(ctx context.Context, id string) (Snapshot, error) {
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	e, err := s.load(id)
	if err != nil {
		return Snapshot{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return snapshot(e.state)
}
func (s *Service) List(ctx context.Context) ([]GameSummary, error) {
	files, err := s.store.List("games")
	if err != nil {
		return nil, err
	}
	result := []GameSummary{}
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		if len(result) >= 1000 {
			return nil, fmt.Errorf("session capacity reached")
		}
		x, err := s.Get(ctx, strings.TrimSuffix(file.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		result = append(result, x.GameSummary)
	}
	slices.SortFunc(result, func(a, b GameSummary) int { return strings.Compare(b.UpdatedAt, a.UpdatedAt) })
	return result, nil
}
func (s *Service) Create(ctx context.Context, req CreateRequest) (Snapshot, error) {
	if !library.ValidCommandID(req.CommandID) || req.ExpectedVersion != 0 || !req.Human.Valid() {
		return Snapshot{}, ErrInvalid
	}
	s.createMu.Lock()
	defer s.createMu.Unlock()
	fp := fingerprint("create", req)
	if e, err := s.load(req.CommandID); err == nil {
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.state.CreationHash != fp {
			return Snapshot{}, ErrConflict
		}
		return snapshot(e.state)
	} else if !errors.Is(err, ErrNotFound) {
		return Snapshot{}, err
	}
	files, err := s.store.List("games")
	if err != nil {
		return Snapshot{}, err
	}
	if len(files) >= 1000 {
		return Snapshot{}, fmt.Errorf("%w: session capacity reached", ErrInvalid)
	}
	bot, err := s.library.Get(req.BotID)
	if err != nil {
		return Snapshot{}, err
	}
	seed, err := s.seed()
	if err != nil {
		return Snapshot{}, err
	}
	rng := random.New(seed, "session/opening", 0)
	opening := []game.Dice{}
	for i := 0; i < replay.MaxEvents; i++ {
		d := game.Dice{rng.IntN(6) + 1, rng.IntN(6) + 1}
		opening = append(opening, d)
		if d[0] != d[1] {
			break
		}
	}
	d := opening[len(opening)-1]
	if d[0] == d[1] {
		return Snapshot{}, fmt.Errorf("opening attempt limit reached")
	}
	starter := game.White
	if d[1] > d[0] {
		starter = game.Black
	}
	p := game.Initial(starter)
	bots := [2]string{"human", "human"}
	bots[1-req.Human] = req.BotID
	now := timestamp()
	st := session{FormatVersion: 1, GameSummary: GameSummary{ID: req.CommandID, Version: 1, BotID: req.BotID, BotName: bot.Name, Human: req.Human, Phase: Moving, CreatedAt: now, UpdatedAt: now}, Position: p, Dice: &d, Draft: []game.Step{}, Record: replay.New(p, opening, bots, seed, 0, replay.MaxEvents), Seed: seed, CreationHash: fp, Commands: map[string]library.Receipt{req.CommandID: {Hash: fp, Version: 1}}}
	st.SideTurns[starter] = 1
	if starter != req.Human {
		if err := s.botTurn(ctx, &st); err != nil {
			return Snapshot{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	data, err := json.Marshal(st)
	if err != nil {
		return Snapshot{}, err
	}
	if err := s.store.WriteNew(path(st.ID), data); err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	s.entries[st.ID] = &entry{state: st}
	s.mu.Unlock()
	return snapshot(st)
}
func (s *Service) roll(st *session) {
	side := st.Position.Turn
	rng := random.New(st.Seed, fmt.Sprintf("session/dice/%d", side), st.SideTurns[side])
	d := game.Dice{rng.IntN(6) + 1, rng.IntN(6) + 1}
	st.Dice = &d
	st.SideTurns[side]++
	st.Phase = Moving
}
func (s *Service) apply(st *session, turn game.Turn) error {
	if len(st.Record.Events) >= replay.MaxEvents {
		return fmt.Errorf("%w: full-turn limit reached", ErrInvalid)
	}
	next, err := game.ApplyTurn(st.Position, *st.Dice, turn)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	st.Record.Append(*st.Dice, turn, next)
	st.Position = next
	st.Dice = nil
	st.Draft = []game.Step{}
	if result, terminal := game.Result(next); terminal {
		st.Phase = Finished
		st.Outcome = &result
		st.Record.Finish(next, replay.Completed)
	} else {
		st.Phase = AwaitingRoll
	}
	return nil
}
func (s *Service) botTurn(ctx context.Context, st *session) error {
	if st.Dice == nil {
		s.roll(st)
	}
	source := random.New(st.Seed, fmt.Sprintf("session/agent/%d", st.Position.Turn), st.SideTurns[st.Position.Turn]-1)
	bot, err := s.library.Agent(st.BotID, source)
	if err != nil {
		return err
	}
	actions, err := game.LegalActions(st.Position, *st.Dice)
	if err != nil {
		return err
	}
	index, err := bot.Choose(ctx, st.Position, *st.Dice, actions)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(actions) {
		return fmt.Errorf("invalid bot action")
	}
	return s.apply(st, actions[index].Turn)
}
func clone(st session) session {
	st.Draft = append([]game.Step{}, st.Draft...)
	st.Record.Events = append([]replay.Event{}, st.Record.Events...)
	commands := make(map[string]library.Receipt, len(st.Commands)+1)
	for k, v := range st.Commands {
		commands[k] = v
	}
	st.Commands = commands
	return st
}
func (s *Service) mutate(ctx context.Context, id, op string, cmd Command, req any, change func(*session) error) (Snapshot, error) {
	if !library.ValidCommandID(cmd.CommandID) {
		return Snapshot{}, ErrInvalid
	}
	e, err := s.load(id)
	if err != nil {
		return Snapshot{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	fp := fingerprint(op, req)
	if receipt, ok := e.state.Commands[cmd.CommandID]; ok {
		if receipt.Hash != fp {
			return Snapshot{}, ErrConflict
		}
		if err := s.archive(e.state); err != nil {
			return Snapshot{}, err
		}
		return snapshot(e.state)
	}
	if e.state.Version != cmd.ExpectedVersion {
		return Snapshot{}, ErrConflict
	}
	if len(e.state.Commands) >= 20000 {
		return Snapshot{}, fmt.Errorf("%w: command limit reached", ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	st := clone(e.state)
	if err := change(&st); err != nil {
		return Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	st.Version++
	st.UpdatedAt = timestamp()
	st.Commands[cmd.CommandID] = library.Receipt{Hash: fp, Version: st.Version}
	data, err := json.Marshal(st)
	if err != nil {
		return Snapshot{}, err
	}
	if len(data) > replay.MaxBytes {
		return Snapshot{}, fmt.Errorf("session size limit reached")
	}
	if err := s.store.WriteAtomic(path(id), data); err != nil {
		return Snapshot{}, err
	}
	e.state = st
	if err := s.archive(st); err != nil {
		return Snapshot{}, err
	}
	return snapshot(st)
}
func (s *Service) Roll(ctx context.Context, id string, cmd Command) (Snapshot, error) {
	return s.mutate(ctx, id, "roll", cmd, cmd, func(st *session) error {
		if st.Phase != AwaitingRoll {
			return fmt.Errorf("%w: dice are already assigned or game is finished", ErrInvalid)
		}
		s.roll(st)
		return nil
	})
}
func (s *Service) SetDraft(ctx context.Context, id string, req DraftRequest) (Snapshot, error) {
	return s.mutate(ctx, id, "draft", req.Command, req, func(st *session) error {
		if st.Phase != Moving || st.Dice == nil || len(req.Prefix) > 4 {
			return ErrInvalid
		}
		if _, _, err := game.PreviewTurn(st.Position, *st.Dice, req.Prefix); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		st.Draft = append([]game.Step{}, req.Prefix...)
		return nil
	})
}
func (s *Service) Confirm(ctx context.Context, id string, req TurnRequest) (Snapshot, error) {
	return s.mutate(ctx, id, "turn", req.Command, req, func(st *session) error {
		if st.Phase != Moving || st.Dice == nil || !slices.Equal(st.Draft, req.Turn.Steps) {
			return fmt.Errorf("%w: confirm the current complete draft", ErrInvalid)
		}
		if err := s.apply(st, req.Turn); err != nil {
			return err
		}
		if st.Phase != Finished {
			return s.botTurn(ctx, st)
		}
		return nil
	})
}
func (s *Service) archive(st session) error {
	if st.Phase != Finished {
		return nil
	}
	var b bytes.Buffer
	if err := replay.Encode(&b, st.Record); err != nil {
		return err
	}
	name := filepath.Join("replays", "game-"+st.ID+".json")
	if err := s.store.WriteNew(name, b.Bytes()); err != nil {
		if !os.IsExist(err) {
			return err
		}
		data, err := s.store.Read(name, replay.MaxBytes)
		if err != nil {
			return err
		}
		if !bytes.Equal(data, b.Bytes()) {
			return fmt.Errorf("existing replay differs from session")
		}
	}
	return nil
}
func (s *Service) Replay(ctx context.Context, id string) (replay.Record, error) {
	if err := ctx.Err(); err != nil {
		return replay.Record{}, err
	}
	e, err := s.load(id)
	if err != nil {
		return replay.Record{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state.Phase != Finished {
		return replay.Record{}, ErrConflict
	}
	if err := s.archive(e.state); err != nil {
		return replay.Record{}, err
	}
	// Copy through the validated codec so callers cannot modify authoritative history.
	var b bytes.Buffer
	if err := replay.Encode(&b, e.state.Record); err != nil {
		return replay.Record{}, err
	}
	return replay.Decode(&b)
}
