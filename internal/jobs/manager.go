// Package jobs owns a bounded durable queue shared by the CLI and local API.
package jobs

import (
	"bytes"
	"context"
	"crypto/sha256"
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

	"evonardy/internal/agent"
	"evonardy/internal/arena"
	"evonardy/internal/features"
	"evonardy/internal/game"
	"evonardy/internal/library"
	"evonardy/internal/replay"
	"evonardy/internal/storage"
	"evonardy/internal/training"
)

const Queued = "queued"
const Running = "running"
const Stopping = "stopping"
const Stopped = "stopped"
const Interrupted = "interrupted"
const Completed = "completed"
const Failed = "failed"
const Training = "training"
const Evaluation = "evaluation"

var ErrInvalid = library.ErrInvalid
var ErrConflict = library.ErrConflict
var ErrNotFound = errors.New("job not found")

type StartRequest struct {
	HybridConfig *training.HybridConfig `json:"hybrid_config,omitempty"`
	library.Command
	Name      string             `json:"name"`
	Config    training.Config    `json:"config"`
	Algorithm string             `json:"algorithm,omitempty"`
	TDConfig  *training.TDConfig `json:"td_config,omitempty"`
}
type SaveRequest struct {
	library.Command
	Generation  int    `json:"generation"`
	CandidateID string `json:"candidate_id"`
	Name        string `json:"name"`
}
type EvaluateRequest struct {
	library.Command
	BotID  string           `json:"bot_id"`
	Config EvaluationConfig `json:"config"`
}
type EvaluationConfig struct {
	Seed        uint64   `json:"seed"`
	Pairs       int      `json:"pairs"`
	Workers     int      `json:"workers"`
	MaxTurns    int      `json:"max_turns"`
	OpponentIDs []string `json:"opponent_ids"`
}

func DefaultEvaluationConfig() EvaluationConfig {
	return EvaluationConfig{Seed: 2026, Pairs: 10, Workers: 2, MaxTurns: 1200, OpponentIDs: []string{library.HeuristicID, library.RandomID}}
}
func (c EvaluationConfig) Validate() error {
	if c.Pairs < 1 || c.Pairs > 500 || c.Workers < 1 || c.Workers > 8 || c.MaxTurns < 1 || c.MaxTurns > replay.MaxEvents || len(c.OpponentIDs) < 1 || len(c.OpponentIDs) > 8 || c.Pairs*len(c.OpponentIDs)*2 > 1000 || int64(c.Pairs*len(c.OpponentIDs)*2)*int64(c.MaxTurns) > 5000000 {
		return fmt.Errorf("invalid evaluation budget: up to 1000 games, 5000000 turn slots, and 8 workers")
	}
	seen := map[string]bool{}
	for _, id := range c.OpponentIDs {
		if id == "" || seen[id] {
			return fmt.Errorf("missing or duplicate opponent")
		}
		seen[id] = true
	}
	return nil
}

type EvaluationSummary struct {
	BotID  string           `json:"bot_id"`
	Config EvaluationConfig `json:"config"`
	Stats  *training.Stats  `json:"stats"`
	Scores []training.Score `json:"scores"`
}
type Publication struct {
	BotID       string `json:"bot_id"`
	Generation  int    `json:"generation"`
	CandidateID string `json:"candidate_id"`
}
type Snapshot struct {
	Execution          *ExecutionInfo          `json:"execution,omitempty"`
	HybridConfig       *training.HybridConfig  `json:"hybrid_config,omitempty"`
	PopulationProgress *PopulationProgress     `json:"population_progress,omitempty"`
	NeuralCandidates   []training.NeuroSummary `json:"neural_candidates,omitempty"`
	Replacements       []training.Replacement  `json:"replacements,omitempty"`
	ID                 string                  `json:"id"`
	Kind               string                  `json:"kind"`
	Name               string                  `json:"name"`
	Version            uint64                  `json:"version"`
	Revision           uint64                  `json:"revision"`
	State              string                  `json:"state"`
	CreatedAt          string                  `json:"created_at"`
	UpdatedAt          string                  `json:"updated_at"`
	Error              string                  `json:"error"`
	CanResume          bool                    `json:"can_resume"`
	Config             *training.Config        `json:"config"`
	Algorithm          string                  `json:"algorithm,omitempty"`
	TDConfig           *training.TDConfig      `json:"td_config,omitempty"`
	TDHistory          []training.TDMetric     `json:"td_history,omitempty"`
	NeuralCandidate    *NeuralCandidate        `json:"neural_candidate,omitempty"`
	Generation         int                     `json:"generation"`
	GenerationGames    int                     `json:"generation_games"`
	GenerationBudget   int                     `json:"generation_budget"`
	Counters           training.Counters       `json:"counters"`
	History            []training.Metric       `json:"history"`
	Candidates         []training.Candidate    `json:"candidates"`
	Saved              []Publication           `json:"saved"`
	WallSeconds        float64                 `json:"wall_seconds"`
	Evaluation         *EvaluationSummary      `json:"evaluation"`
	WatchedGame        *GameNotice             `json:"watched_game,omitempty"`
}
type receipt struct {
	Hash    string `json:"hash"`
	Version uint64 `json:"version"`
	BotID   string `json:"bot_id"`
}
type evaluationState struct {
	Algorithm       string           `json:"algorithm"`
	Ruleset         string           `json:"ruleset"`
	FeaturesVersion string           `json:"features_version"`
	Config          EvaluationConfig `json:"config"`
	Target          agent.Policy     `json:"target"`
	Opponents       []agent.Policy   `json:"opponents"`
	Results         []training.Score `json:"results"`
}
type record struct {
	NeuroTraining *training.NeuroState `json:"neuro_training,omitempty"`
	Snapshot
	FormatVersion    int                  `json:"format_version"`
	CreationHash     string               `json:"creation_hash"`
	Commands         map[string]receipt   `json:"commands"`
	Training         *training.State      `json:"training"`
	TDTraining       *training.TDState    `json:"td_training,omitempty"`
	FrozenEvaluation *evaluationState     `json:"frozen_evaluation"`
	GenerationHashes []string             `json:"generation_hashes"`
	WatchReplay      *training.PlayedGame `json:"watch_replay,omitempty"`
}
type envelope struct {
	Checksum string          `json:"checksum"`
	Record   json.RawMessage `json:"record"`
}
type Manager struct {
	mu      sync.Mutex
	store   *storage.Store
	bots    *library.Library
	records map[string]record
	wake    chan struct{}
	changed chan struct{}
	done    chan struct{}
	closing bool
	failure error
}

func New(store *storage.Store, bots *library.Library) (*Manager, error) {
	m := &Manager{store: store, bots: bots, records: map[string]record{}, wake: make(chan struct{}, 1), changed: make(chan struct{}), done: make(chan struct{})}
	for _, base := range []string{"runs", "evaluations"} {
		entries, err := store.List(base)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() || !library.ValidCommandID(entry.Name()) {
				continue
			}
			path := filepath.Join(base, entry.Name(), "checkpoint.json")
			data, err := store.Read(path, replay.MaxBytes)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, err
			}
			var e envelope
			if err := library.DecodeJSON(data, &e); err != nil {
				return nil, err
			}
			if digest(e.Record) != e.Checksum {
				return nil, fmt.Errorf("checkpoint checksum mismatch: %s", entry.Name())
			}
			var r record
			if err := library.DecodeJSON(e.Record, &r); err != nil {
				return nil, err
			}
			if err := validate(r); err != nil {
				return nil, fmt.Errorf("checkpoint %s: %w", entry.Name(), err)
			}
			if r.ID != entry.Name() || recordPath(r) != path {
				return nil, fmt.Errorf("checkpoint identity mismatch")
			}
			if _, exists := m.records[r.ID]; exists {
				return nil, fmt.Errorf("duplicate job identity")
			}
			if len(m.records) >= 200 {
				return nil, fmt.Errorf("job capacity reached")
			}
			if r.State == Running || r.State == Queued || r.State == Stopping {
				r.State = Interrupted
				r.Error = "Process interrupted; resume from the durable checkpoint"
				r.Revision++
				if err := m.persist(r); err != nil {
					return nil, err
				}
			} else {
				m.records[r.ID] = r
			}
		}
	}
	go m.loop()
	return m, nil
}
func digest(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func fingerprint(op string, req any) string {
	data, _ := json.Marshal(req)
	return digest(append([]byte(op+":"), data...))
}
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func recordPath(r record) string {
	base := "runs"
	if r.Kind == Evaluation {
		base = "evaluations"
	}
	return filepath.Join(base, r.ID, "checkpoint.json")
}
func generationPath(id string, g int) string {
	return filepath.Join("runs", id, "generations", fmt.Sprintf("%04d.json", g))
}
func syncSnapshot(r *record) {
	r.WatchedGame = gameNotice(r.WatchReplay)
	r.CanResume = r.State == Stopped || r.State == Interrupted
	if r.NeuroTraining != nil {
		syncNeuroSnapshot(r)
	}
	if r.TDTraining != nil {
		syncTDSnapshot(r)
	}
	if r.Training != nil {
		st := r.Training
		c := st.Config
		r.Config = &c
		r.Generation = st.Generation
		r.Counters = st.Counters
		r.History = st.History
		r.GenerationGames = len(st.Results)
		r.GenerationBudget = training.Slots(*st)
	}
	if r.FrozenEvaluation != nil {
		e := r.FrozenEvaluation
		r.GenerationGames = len(e.Results)
		r.GenerationBudget = e.Config.Pairs * len(e.Opponents) * 2
		r.Evaluation = &EvaluationSummary{BotID: e.Target.ID, Config: e.Config, Scores: e.Results}
		if r.State == Completed {
			stats, err := training.Summarize(e.Results)
			if err == nil {
				r.Evaluation.Stats = &stats
			}
		}
	}
}
func (m *Manager) persist(r record) error {
	syncSnapshot(&r)
	r.UpdatedAt = now()
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	data, err = json.Marshal(envelope{Checksum: digest(data), Record: data})
	if err != nil {
		return err
	}
	if len(data) > replay.MaxBytes {
		return fmt.Errorf("checkpoint exceeds 16 MiB")
	}
	if err := m.store.WriteAtomic(recordPath(r), data); err != nil {
		return err
	}
	m.records[r.ID] = r
	close(m.changed)
	m.changed = make(chan struct{})
	return nil
}
func clone(r record) record {
	commands := make(map[string]receipt, len(r.Commands)+1)
	for k, v := range r.Commands {
		commands[k] = v
	}
	r.Commands = commands
	return r
}
func public(r record) Snapshot {
	data, _ := json.Marshal(r.Snapshot)
	var x Snapshot
	_ = json.Unmarshal(data, &x)
	return x
}
func (m *Manager) Get(id string) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return Snapshot{}, ErrNotFound
	}
	return public(r), nil
}

// Status is the small revision notice used by throttled SSE streams.
type Status struct {
	ID         string `json:"run_id"`
	Kind       string `json:"kind"`
	State      string `json:"state"`
	Version    uint64 `json:"version"`
	Revision   uint64 `json:"revision"`
	Generation int    `json:"generation"`
}

func (m *Manager) Status(id string) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return Status{}, ErrNotFound
	}
	return Status{r.ID, r.Kind, r.State, r.Version, r.Revision, r.Generation}, nil
}
func (m *Manager) List(kind string) ([]Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	xs := []Snapshot{}
	for _, r := range m.records {
		if kind == "" || r.Kind == kind {
			// Lists carry summaries; large generation and game records belong to GET.
			r.Candidates = []training.Candidate{}
			r.History = []training.Metric{}
			r.TDHistory = nil
			r.NeuralCandidates = nil
			r.Replacements = nil
			r.Saved = []Publication{}
			if r.Evaluation != nil {
				e := *r.Evaluation
				e.Scores = []training.Score{}
				r.Evaluation = &e
			}
			xs = append(xs, public(r))
		}
	}
	slices.SortFunc(xs, func(a, b Snapshot) int { return strings.Compare(b.CreatedAt, a.CreatedAt) })
	return xs, nil
}
func (m *Manager) notify() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func initial(id, kind, name, hash string) record {
	timestamp := now()
	return record{Snapshot: Snapshot{ID: id, Kind: kind, Name: name, Version: 1, Revision: 1, State: Queued, CreatedAt: timestamp, History: []training.Metric{}, Candidates: []training.Candidate{}, Saved: []Publication{}}, FormatVersion: 1, CreationHash: hash, Commands: map[string]receipt{id: {Hash: hash, Version: 1}}, GenerationHashes: []string{}}
}
func (m *Manager) existing(id, fp string) (Snapshot, bool, error) {
	if r, ok := m.records[id]; ok {
		if r.CreationHash != fp {
			return Snapshot{}, true, ErrConflict
		}
		return public(r), true, nil
	}
	return Snapshot{}, false, nil
}
func (m *Manager) Start(ctx context.Context, req StartRequest) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fp := fingerprint("start", req)
	if x, ok, err := m.existing(req.CommandID, fp); ok {
		return x, err
	}
	if m.closing || !library.ValidCommandID(req.CommandID) || req.ExpectedVersion != 0 || strings.TrimSpace(req.Name) == "" || len(req.Name) > 128 || len(m.records) >= 200 {
		return Snapshot{}, fmt.Errorf("%w: invalid run name, command, or capacity", ErrInvalid)
	}
	r := initial(req.CommandID, Training, strings.TrimSpace(req.Name), fp)
	if req.HybridConfig != nil || req.Algorithm == training.GAMLPAlgorithm {
		if req.TDConfig != nil || (req.HybridConfig != nil && (req.Config != (training.Config{}) || req.Algorithm != training.HybridAlgorithm)) {
			return Snapshot{}, fmt.Errorf("%w: incompatible population configuration", ErrInvalid)
		}
		var st training.NeuroState
		var err error
		if req.HybridConfig != nil {
			st, err = training.NewHybrid(*req.HybridConfig)
		} else {
			st, err = training.NewGAMLP(req.Config)
		}
		if err != nil {
			return Snapshot{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		r.NeuroTraining = &st
		r.Execution = neuroExecution(st)
	} else if req.TDConfig != nil {
		if req.Config != (training.Config{}) || req.TDConfig.Games > maxTDJobGames || (req.Algorithm != "" && req.Algorithm != req.TDConfig.Algorithm()) {
			return Snapshot{}, fmt.Errorf("%w: incompatible neural configuration or game budget", ErrInvalid)
		}
		st, err := training.NewTD(*req.TDConfig)
		if err != nil {
			return Snapshot{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		r.TDTraining = &st
	} else {
		if req.Algorithm != "" && req.Algorithm != training.Algorithm {
			return Snapshot{}, fmt.Errorf("%w: unsupported training algorithm", ErrInvalid)
		}
		st, err := training.New(req.Config)
		if err != nil {
			return Snapshot{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		r.Training = &st
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := m.persist(r); err != nil {
		return Snapshot{}, err
	}
	m.notify()
	return public(m.records[r.ID]), nil
}
func (m *Manager) Evaluate(ctx context.Context, req EvaluateRequest) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fp := fingerprint("evaluate", req)
	if x, ok, err := m.existing(req.CommandID, fp); ok {
		return x, err
	}
	if m.closing || !library.ValidCommandID(req.CommandID) || req.ExpectedVersion != 0 || len(m.records) >= 200 {
		return Snapshot{}, ErrInvalid
	}
	if err := req.Config.Validate(); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	target, err := m.bots.Freeze(req.BotID)
	if err != nil {
		return Snapshot{}, err
	}
	e := evaluationState{Algorithm: "paired-evaluation-v1", Ruleset: game.Ruleset, FeaturesVersion: features.Version, Config: req.Config, Target: target, Opponents: []agent.Policy{}, Results: []training.Score{}}
	e.Config.OpponentIDs = append([]string{}, req.Config.OpponentIDs...)
	for _, id := range req.Config.OpponentIDs {
		p, err := m.bots.Freeze(id)
		if err != nil {
			return Snapshot{}, err
		}
		e.Opponents = append(e.Opponents, p)
	}
	r := initial(req.CommandID, Evaluation, "Independent evaluation", fp)
	r.FrozenEvaluation = &e
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := m.persist(r); err != nil {
		return Snapshot{}, err
	}
	m.notify()
	return public(m.records[r.ID]), nil
}
func command(r record, op string, cmd library.Command, req any) (record, bool, error) {
	if !library.ValidCommandID(cmd.CommandID) {
		return r, false, ErrInvalid
	}
	fp := fingerprint(op, req)
	if old, ok := r.Commands[cmd.CommandID]; ok {
		if old.Hash != fp {
			return r, false, ErrConflict
		}
		return r, true, nil
	}
	if cmd.ExpectedVersion != r.Version {
		return r, false, ErrConflict
	}
	if len(r.Commands) >= 4096 {
		return r, false, fmt.Errorf("%w: control receipt limit reached", ErrInvalid)
	}
	r = clone(r)
	r.Version++
	r.Revision++
	r.Commands[cmd.CommandID] = receipt{Hash: fp, Version: r.Version}
	return r, false, nil
}
func (m *Manager) control(ctx context.Context, id, op string, cmd library.Command) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return Snapshot{}, ErrNotFound
	}
	next, duplicate, err := command(r, op, cmd, cmd)
	if err != nil {
		return Snapshot{}, err
	}
	if duplicate {
		return public(r), nil
	}
	if m.closing {
		return Snapshot{}, ErrInvalid
	}
	switch op {
	case "stop":
		if r.State != Queued && r.State != Running && r.State != Stopping {
			return Snapshot{}, fmt.Errorf("%w: job is not active", ErrInvalid)
		}
		next.State = Stopping
		if r.State == Queued {
			next.State = Stopped
		}
		next.Error = "Stopped by user; checkpoint ready"
	case "resume":
		if !r.CanResume {
			return Snapshot{}, fmt.Errorf("%w: job cannot resume", ErrInvalid)
		}
		next.State = Queued
		next.Error = ""
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := m.persist(next); err != nil {
		return Snapshot{}, err
	}
	m.notify()
	return public(m.records[id]), nil
}
func (m *Manager) Stop(ctx context.Context, id string, cmd library.Command) (Snapshot, error) {
	return m.control(ctx, id, "stop", cmd)
}
func (m *Manager) Resume(ctx context.Context, id string, cmd library.Command) (Snapshot, error) {
	return m.control(ctx, id, "resume", cmd)
}
func (m *Manager) Save(ctx context.Context, id string, req SaveRequest) (library.Card, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return library.Card{}, ErrNotFound
	}
	next, duplicate, err := command(r, "save", req.Command, req)
	if err != nil {
		return library.Card{}, err
	}
	if duplicate {
		detail, err := m.bots.Get(r.Commands[req.CommandID].BotID)
		return detail.Card, err
	}
	if r.NeuroTraining != nil {
		return m.saveNeuro(ctx, r, next, req)
	}
	if r.TDTraining != nil {
		return m.saveTD(ctx, r, next, req)
	}
	if r.Kind != Training || req.Generation < 1 || req.Generation > len(r.GenerationHashes) {
		return library.Card{}, fmt.Errorf("%w: choose an evaluated generation", ErrInvalid)
	}
	gen, err := m.generation(r, req.Generation)
	if err != nil {
		return library.Card{}, err
	}
	var candidate *training.Candidate
	for _, c := range gen.Ranked {
		if c.ID == req.CandidateID {
			copy := c
			candidate = &copy
			break
		}
	}
	if candidate == nil {
		return library.Card{}, fmt.Errorf("%w: candidate not found", ErrInvalid)
	}
	if err := ctx.Err(); err != nil {
		return library.Card{}, err
	}
	card, err := m.bots.SaveLinear(req.Name, candidate.Weights[:], fmt.Sprintf("GA-linear run %s, generation %d", id, req.Generation), id+"/"+candidate.ID)
	if err != nil {
		return library.Card{}, err
	}
	entry := next.Commands[req.CommandID]
	entry.BotID = card.ID
	next.Commands[req.CommandID] = entry
	next.Saved = append(append([]Publication{}, r.Saved...), Publication{BotID: card.ID, Generation: req.Generation, CandidateID: candidate.ID})
	if err := m.persist(next); err != nil {
		return library.Card{}, err
	}
	return card, nil
}
func (m *Manager) generation(r record, n int) (training.Generation, error) {
	var gen training.Generation
	data, err := m.store.Read(generationPath(r.ID, n), replay.MaxBytes)
	if err != nil {
		return gen, err
	}
	if digest(data) != r.GenerationHashes[n-1] {
		return gen, fmt.Errorf("generation checksum mismatch")
	}
	if err := library.DecodeJSON(data, &gen); err != nil {
		return gen, err
	}
	if gen.Number != n || gen.Algorithm != training.Algorithm || gen.Ruleset != game.Ruleset || gen.FeaturesVersion != features.Version {
		return gen, fmt.Errorf("incompatible generation archive")
	}
	return gen, nil
}
func (m *Manager) Generation(id string, n int) (training.Generation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return training.Generation{}, ErrNotFound
	}
	if n < 1 || n > len(r.GenerationHashes) {
		return training.Generation{}, ErrNotFound
	}
	if r.NeuroTraining != nil {
		return m.neuroGenerationView(r, n)
	}
	if r.TDTraining != nil {
		return training.Generation{}, fmt.Errorf("%w: neural runs archive game checkpoints, not evaluated generations", ErrInvalid)
	}
	return m.generation(r, n)
}
func (m *Manager) Wait(ctx context.Context, id string) (Snapshot, error) {
	for {
		m.mu.Lock()
		r, ok := m.records[id]
		ch := m.changed
		if !ok {
			m.mu.Unlock()
			return Snapshot{}, ErrNotFound
		}
		if terminal(r.State) {
			x := public(r)
			m.mu.Unlock()
			return x, nil
		}
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return Snapshot{}, ctx.Err()
		case <-ch:
		}
	}
}
func terminal(state string) bool {
	return state == Stopped || state == Interrupted || state == Failed || state == Completed
}
func (m *Manager) Close() error {
	m.mu.Lock()
	m.closing = true
	m.mu.Unlock()
	m.notify()
	<-m.done
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failure
}
func (m *Manager) loop() {
	defer close(m.done)
	for {
		m.mu.Lock()
		if m.closing {
			m.mu.Unlock()
			return
		}
		var chosen *record
		for _, r := range m.records {
			if r.State == Queued && (chosen == nil || r.CreatedAt < chosen.CreatedAt || (r.CreatedAt == chosen.CreatedAt && r.ID < chosen.ID)) {
				copy := r
				chosen = &copy
			}
		}
		if chosen == nil {
			m.mu.Unlock()
			<-m.wake
			continue
		}
		r := *chosen
		r.State = Running
		r.Revision++
		err := m.persist(r)
		if err != nil {
			m.storageFailure(r.ID, err)
			m.mu.Unlock()
			continue
		}
		m.mu.Unlock()
		m.run(r.ID)
	}
}
func (m *Manager) storageFailure(id string, err error) {
	r := m.records[id]
	r.State = Failed
	r.Error = "Checkpoint write failed; last durable checkpoint preserved"
	r.Revision++
	r.CanResume = false
	m.records[id] = r
	m.failure = errors.Join(m.failure, err)
	close(m.changed)
	m.changed = make(chan struct{})
}
func (m *Manager) run(id string) {
	m.mu.Lock()
	isTD := m.records[id].TDTraining != nil
	isNeuro := m.records[id].NeuroTraining != nil
	m.mu.Unlock()
	if isNeuro {
		m.runNeuro(id)
		return
	}
	if isTD {
		m.runTD(id)
		return
	}
	for {
		m.mu.Lock()
		r := m.records[id]
		if m.closing || r.State == Stopping {
			if m.closing {
				r.State = Interrupted
				r.Error = "Server stopped; checkpoint ready"
			} else {
				r.State = Stopped
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
		var scores []training.Score
		var watched *training.PlayedGame
		var err error
		if r.Kind == Training {
			var wave training.Wave
			wave, err = training.PlayWave(context.Background(), *r.Training, training.NextTasks(*r.Training))
			scores, watched = wave.Scores, wave.Sample
		} else {
			scores, err = playEvaluation(*r.FrozenEvaluation)
		}
		m.mu.Lock()
		current := m.records[id]
		current.Revision++
		if err == nil && current.Kind == Training {
			current.WatchReplay = watched
			var state training.State
			state, err = training.Add(*current.Training, scores)
			current.Training = &state
			if err == nil && training.Ready(state) {
				var gen training.Generation
				state, gen, err = training.Advance(state)
				if err == nil {
					data, encodeErr := json.Marshal(gen)
					err = encodeErr
					if err == nil {
						err = m.store.WriteNew(generationPath(id, gen.Number), data)
						if os.IsExist(err) {
							old, readErr := m.store.Read(generationPath(id, gen.Number), replay.MaxBytes)
							err = readErr
							if err == nil && !bytes.Equal(data, old) {
								err = fmt.Errorf("existing generation differs")
							}
						}
					}
					if err == nil {
						current.GenerationHashes = append(append([]string{}, current.GenerationHashes...), digest(data))
						current.Candidates = gen.Ranked
						current.Training = &state
					}
				}
			}
			if err == nil && current.Training.Generation == current.Training.Config.Generations {
				current.State = Completed
				current.Error = ""
			}
		} else if err == nil {
			e := *current.FrozenEvaluation
			e.Results = append(append([]training.Score{}, e.Results...), scores...)
			current.FrozenEvaluation = &e
			for _, score := range scores {
				current.Counters.Games++
				if score.Status == replay.Completed {
					current.Counters.CompletedGames++
				} else {
					current.Counters.TruncatedGames++
				}
				current.Counters.Decisions += uint64(score.Turns)
				current.Counters.ForwardEvaluations += uint64(score.ForwardEvaluations)
				if score.Status == replay.Truncated {
					err = fmt.Errorf("evaluation game %d truncated; no aggregate result", score.Index)
				}
			}
			if err == nil && len(e.Results) == e.Config.Pairs*len(e.Opponents)*2 {
				current.State = Completed
				current.Error = ""
			}
		}
		if err != nil {
			current.State = Failed
			current.Error = err.Error()
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
func playEvaluation(e evaluationState) ([]training.Score, error) {
	total := e.Config.Pairs * len(e.Opponents) * 2
	n := min(e.Config.Workers, total-len(e.Results))
	if n < 1 {
		return nil, fmt.Errorf("evaluation already complete")
	}
	scores := make([]training.Score, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			index := len(e.Results) + i
			opponentIndex := index / (e.Config.Pairs * 2)
			pair := (index / 2) % e.Config.Pairs
			side := game.Player(index % 2)
			opponent := e.Opponents[opponentIndex]
			seed := training.PairSeed(e.Config.Seed, "evaluation/independent-v1", opponentIndex, pair)
			policies := [2]arena.Factory{}
			policies[side] = arena.Factory{ID: e.Target.ID, New: e.Target.New}
			policies[side.Other()] = arena.Factory{ID: opponent.ID, New: opponent.New}
			result, err := arena.Play(context.Background(), arena.MatchConfig{Seed: seed, ID: uint64(index), StreamID: 0, MaxTurns: e.Config.MaxTurns}, policies)
			errs[i] = err
			if err == nil {
				scores[i] = training.Score{Index: index, Seed: seed, Side: side, OpponentID: opponent.ID, Status: result.Record.Status, Outcome: result.Record.Outcome, Turns: result.Decisions, ForwardEvaluations: result.ForwardEvaluations}
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return scores, nil
}
func validate(r record) error {
	if r.FormatVersion != 1 || !library.ValidCommandID(r.ID) || r.Version < 1 || r.Revision < 1 || r.Commands == nil || len(r.Commands) > 4096 {
		return fmt.Errorf("incompatible job checkpoint")
	}
	switch r.State {
	case Queued, Running, Stopping, Stopped, Interrupted, Completed, Failed:
	default:
		return fmt.Errorf("invalid job state")
	}
	if r.Kind == Training {
		if r.NeuroTraining != nil {
			return validateNeuroRecord(r)
		}
		if r.TDTraining != nil {
			return validateTDRecord(r)
		}
		if r.Training == nil || r.FrozenEvaluation != nil || len(r.GenerationHashes) != r.Training.Generation {
			return fmt.Errorf("invalid training archive references")
		}
		if err := training.Validate(*r.Training); err != nil {
			return err
		}
		if err := validateWatch(r); err != nil {
			return err
		}
		if r.State == Completed && r.Training.Generation != r.Training.Config.Generations {
			return fmt.Errorf("incomplete training marked completed")
		}
	} else if r.Kind == Evaluation {
		if r.WatchReplay != nil || r.WatchedGame != nil {
			return fmt.Errorf("training replay in evaluation checkpoint")
		}
		e := r.FrozenEvaluation
		if e == nil || r.NeuroTraining != nil || r.Training != nil || r.TDTraining != nil || e.Algorithm != "paired-evaluation-v1" || e.Ruleset != game.Ruleset || e.FeaturesVersion != features.Version {
			return fmt.Errorf("incompatible evaluation checkpoint")
		}
		if err := e.Config.Validate(); err != nil {
			return err
		}
		if err := e.Target.Validate(); err != nil {
			return err
		}
		if len(e.Opponents) != len(e.Config.OpponentIDs) || len(e.Results) > e.Config.Pairs*len(e.Opponents)*2 {
			return fmt.Errorf("invalid evaluation dimensions")
		}
		for i, p := range e.Opponents {
			if err := p.Validate(); err != nil {
				return err
			}
			if p.ID != e.Config.OpponentIDs[i] {
				return fmt.Errorf("evaluation opponent mismatch")
			}
		}
		for i, score := range e.Results {
			op := i / (e.Config.Pairs * 2)
			pair := (i / 2) % e.Config.Pairs
			if score.Index != i || score.Side != game.Player(i%2) || score.OpponentID != e.Opponents[op].ID || score.Seed != training.PairSeed(e.Config.Seed, "evaluation/independent-v1", op, pair) {
				return fmt.Errorf("evaluation schedule mismatch")
			}
			if err := training.ValidScore(score, e.Config.MaxTurns); err != nil {
				return err
			}
		}
		var completed, truncated, decisions, forwards uint64
		for _, score := range e.Results {
			if score.Status == replay.Completed {
				completed++
			} else {
				truncated++
			}
			decisions += uint64(score.Turns)
			forwards += uint64(score.ForwardEvaluations)
		}
		if r.Counters.Games != uint64(len(e.Results)) || r.Counters.CompletedGames != completed || r.Counters.TruncatedGames != truncated || r.Counters.Decisions != decisions || r.Counters.ForwardEvaluations != forwards || r.Counters.Mutations != 0 || r.Counters.Crossovers != 0 || r.Counters.Updates != 0 {
			return fmt.Errorf("evaluation counters mismatch")
		}
		if r.State == Completed {
			if len(e.Results) != e.Config.Pairs*len(e.Opponents)*2 {
				return fmt.Errorf("incomplete evaluation marked completed")
			}
			if _, err := training.Summarize(e.Results); err != nil {
				return err
			}
		}
	} else {
		return fmt.Errorf("unknown job kind")
	}
	return nil
}
