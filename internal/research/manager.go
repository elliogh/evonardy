package research

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"time"

	"evonardy/internal/agent"
	"evonardy/internal/encoder"
	"evonardy/internal/game"
	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/neural"
	"evonardy/internal/replay"
	"evonardy/internal/storage"
	"evonardy/internal/training"
)

var ErrNotFound = errors.New("experiment not found")

type StartRequest struct {
	library.Command
	Name   string `json:"name"`
	Config Config `json:"config"`
}
type Execution struct {
	TDContract         string `json:"td_contract"`
	PopulationContract string `json:"population_contract"`
	Platform           string `json:"platform"`
	GoVersion          string `json:"go_version"`
	SourceRevision     string `json:"source_revision"`
	SourceModified     bool   `json:"source_modified"`
	Ruleset            string `json:"ruleset"`
	Encoder            string `json:"encoder"`
	Network            string `json:"network"`
	Schedule           string `json:"schedule"`
	Workers            int    `json:"workers"`
}
type PolicyIdentity struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}
type Run struct {
	Algorithm           string            `json:"algorithm"`
	Seed                uint64            `json:"seed"`
	Config              Method            `json:"config"`
	BotID               string            `json:"bot_id"`
	Manifest            *library.Manifest `json:"manifest"`
	Counters            training.Counters `json:"counters"`
	TrainingGames       uint64            `json:"training_games"`
	SelectionGames      uint64            `json:"selection_games"`
	WallSeconds         float64           `json:"wall_seconds"`
	DevelopmentSeconds  float64           `json:"development_seconds"`
	DevelopmentCounters training.Counters `json:"development_counters"`
	Scores              []training.Score  `json:"scores"`
	Estimate            *Estimate         `json:"estimate"`
}
type MethodResult struct {
	Algorithm string   `json:"algorithm"`
	Estimate  Estimate `json:"estimate"`
}
type Difference struct {
	A        string   `json:"a"`
	B        string   `json:"b"`
	Estimate Estimate `json:"estimate"`
}
type Snapshot struct {
	FinalDataRole       string            `json:"final_data_role"`
	FinalDataReusedFrom []string          `json:"final_data_reused_from"`
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	Version             uint64            `json:"version"`
	Revision            uint64            `json:"revision"`
	State               string            `json:"state"`
	Phase               string            `json:"phase"`
	Error               string            `json:"error"`
	CanResume           bool              `json:"can_resume"`
	CreatedAt           string            `json:"created_at"`
	UpdatedAt           string            `json:"updated_at"`
	Config              Config            `json:"config"`
	Execution           Execution         `json:"execution"`
	FrozenOpponents     []PolicyIdentity  `json:"frozen_opponents"`
	FrozenIncumbent     PolicyIdentity    `json:"frozen_incumbent"`
	Runs                []Run             `json:"runs"`
	Active              *Run              `json:"active"`
	Methods             []MethodResult    `json:"methods"`
	Differences         []Difference      `json:"differences"`
	CandidateID         string            `json:"candidate_id"`
	SelectionLocked     bool              `json:"selection_locked"`
	FinalScores         []training.Score  `json:"final_scores"`
	FinalCounters       training.Counters `json:"final_counters"`
	FinalSeconds        float64           `json:"final_seconds"`
	Verdict             *Verdict          `json:"verdict"`
	Counters            training.Counters `json:"counters"`
	WallSeconds         float64           `json:"wall_seconds"`
	ReportSHA256        string            `json:"report_sha256"`
}
type record struct {
	Snapshot
	Format       int                  `json:"format"`
	CreationHash string               `json:"creation_hash"`
	Commands     map[string]string    `json:"commands"`
	Opponents    []agent.Policy       `json:"opponents"`
	Incumbent    agent.Policy         `json:"incumbent"`
	Target       *agent.Policy        `json:"target"`
	TD           *training.TDState    `json:"td_state"`
	Neuro        *training.NeuroState `json:"neuro_state"`
}
type envelope struct {
	SHA256 string          `json:"sha256"`
	Record json.RawMessage `json:"record"`
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

func hash(data []byte) string      { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }
func fingerprint(value any) string { data, _ := json.Marshal(value); return hash(data) }
func timestamp() string            { return time.Now().UTC().Format(time.RFC3339Nano) }
func path(id string) string        { return filepath.Join("experiments", id, "checkpoint.json") }
func clone(r record) record {
	data, _ := json.Marshal(r)
	var out record
	_ = json.Unmarshal(data, &out)
	return out
}
func public(r record) Snapshot { return clone(r).Snapshot }
func execution(workers int) Execution {
	x := Execution{Platform: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(), SourceRevision: "unknown", TDContract: training.TDRandomContract, PopulationContract: training.NeuroRandomContract, Ruleset: game.Ruleset, Encoder: encoder.Version, Network: neural.Version, Schedule: ScheduleVersion, Workers: workers}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				x.SourceRevision = s.Value
			case "vcs.modified":
				x.SourceModified = s.Value == "true"
			}
		}
	}
	return x
}
func identity(p agent.Policy) PolicyIdentity { return PolicyIdentity{ID: p.ID, SHA256: fingerprint(p)} }
func New(store *storage.Store, bots *library.Library) (*Manager, error) {
	m := &Manager{store: store, bots: bots, records: map[string]record{}, wake: make(chan struct{}, 1), changed: make(chan struct{}), done: make(chan struct{})}
	entries, err := store.List("experiments")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !library.ValidCommandID(entry.Name()) {
			continue
		}
		data, err := store.Read(path(entry.Name()), replay.MaxBytes)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var e envelope
		if err = library.DecodeJSON(data, &e); err != nil {
			return nil, err
		}
		if hash(e.Record) != e.SHA256 {
			return nil, fmt.Errorf("experiment checksum mismatch")
		}
		var r record
		if err = library.DecodeJSON(e.Record, &r); err != nil {
			return nil, err
		}
		if r.ID != entry.Name() {
			return nil, fmt.Errorf("experiment identity mismatch")
		}
		if err = validate(r); err != nil {
			return nil, fmt.Errorf("experiment %s: %w", r.ID, err)
		}
		for _, run := range r.Runs {
			detail, err := bots.Get(run.BotID)
			if err != nil {
				return nil, err
			}
			if detail.Manifest == nil || fingerprint(detail.Manifest) != fingerprint(run.Manifest) {
				return nil, fmt.Errorf("experiment model manifest differs")
			}
		}
		if r.Target != nil {
			p, err := bots.Freeze(r.Target.ID)
			if err != nil {
				return nil, err
			}
			if fingerprint(p) != fingerprint(*r.Target) {
				return nil, fmt.Errorf("experiment target differs from immutable model")
			}
		}
		if r.State == jobs.Completed {
			data, err := store.Read(reportPath(r.ID, r.ReportSHA256), replay.MaxBytes)
			if err != nil {
				return nil, err
			}
			if hash(data) != r.ReportSHA256 {
				return nil, fmt.Errorf("experiment report checksum mismatch")
			}
		}
		if len(m.records) >= 100 {
			return nil, fmt.Errorf("experiment capacity reached")
		}
		if r.State == jobs.Running || r.State == jobs.Queued || r.State == jobs.Stopping {
			r.State = jobs.Interrupted
			r.Error = "Process interrupted; resume from durable boundary"
			r.Revision++
			if err = m.persist(r); err != nil {
				return nil, err
			}
		} else {
			m.records[r.ID] = r
		}
	}
	go m.loop()
	return m, nil
}
func (m *Manager) persist(r record) error {
	r.CanResume = r.State == jobs.Stopped || r.State == jobs.Interrupted
	r.UpdatedAt = timestamp()
	r.Counters = training.Counters{}
	r.WallSeconds = r.FinalSeconds
	for _, run := range r.Runs {
		add(&r.Counters, run.Counters)
		add(&r.Counters, run.DevelopmentCounters)
		r.WallSeconds += run.WallSeconds + run.DevelopmentSeconds
	}
	if r.Active != nil {
		add(&r.Counters, r.Active.Counters)
		add(&r.Counters, r.Active.DevelopmentCounters)
		r.WallSeconds += r.Active.WallSeconds + r.Active.DevelopmentSeconds
	}
	add(&r.Counters, r.FinalCounters)
	data, err := json.Marshal(r)
	if err != nil {
		return err
	}
	blob, err := json.Marshal(envelope{SHA256: hash(data), Record: data})
	if err != nil {
		return err
	}
	if len(blob) > replay.MaxBytes {
		return fmt.Errorf("experiment checkpoint size limit reached")
	}
	if err = m.store.WriteAtomic(path(r.ID), append(blob, '\n')); err != nil {
		return err
	}
	m.records[r.ID] = r
	close(m.changed)
	m.changed = make(chan struct{})
	return nil
}
func add(a *training.Counters, b training.Counters) {
	a.Games += b.Games
	a.CompletedGames += b.CompletedGames
	a.TruncatedGames += b.TruncatedGames
	a.Decisions += b.Decisions
	a.ForwardEvaluations += b.ForwardEvaluations
	a.Updates += b.Updates
	a.Mutations += b.Mutations
	a.Crossovers += b.Crossovers
}
func (m *Manager) notify() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
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
func (m *Manager) List() ([]Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []Snapshot{}
	for _, r := range m.records {
		x := public(r)
		x.Runs = nil
		x.Active = nil
		x.Methods = nil
		x.Differences = nil
		x.FinalScores = nil
		out = append(out, x)
	}
	slices.SortFunc(out, func(a, b Snapshot) int { return strings.Compare(b.CreatedAt, a.CreatedAt) })
	return out, nil
}
func (m *Manager) Start(ctx context.Context, req StartRequest) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fp := fingerprint(req)
	if old, ok := m.records[req.CommandID]; ok {
		if old.CreationHash != fp {
			return Snapshot{}, library.ErrConflict
		}
		return public(old), nil
	}
	if m.failure != nil {
		return Snapshot{}, m.failure
	}
	if m.closing || !library.ValidCommandID(req.CommandID) || req.ExpectedVersion != 0 || strings.TrimSpace(req.Name) == "" || len(req.Name) > 128 || len(m.records) >= 100 {
		return Snapshot{}, fmt.Errorf("%w: invalid experiment command, name or capacity", library.ErrInvalid)
	}
	if err := req.Config.Validate(); err != nil {
		return Snapshot{}, fmt.Errorf("%w: %v", library.ErrInvalid, err)
	}
	r := record{Snapshot: Snapshot{ID: req.CommandID, Name: strings.TrimSpace(req.Name), Version: 1, Revision: 1, State: jobs.Queued, Phase: "training", CreatedAt: timestamp(), Config: req.Config, Execution: execution(req.Config.Workers), Runs: []Run{}, Methods: []MethodResult{}, Differences: []Difference{}, FinalScores: []training.Score{}, FrozenOpponents: []PolicyIdentity{}}, Format: 1, CreationHash: fp, Commands: map[string]string{}, Opponents: []agent.Policy{}}
	for _, id := range req.Config.OpponentIDs {
		p, err := m.bots.Freeze(id)
		if err != nil {
			return Snapshot{}, err
		}
		r.Opponents = append(r.Opponents, p)
		r.FrozenOpponents = append(r.FrozenOpponents, identity(p))
	}
	inc, err := m.bots.Freeze(req.Config.IncumbentID)
	if err != nil {
		return Snapshot{}, err
	}
	r.Incumbent = inc
	r.FrozenIncumbent = identity(inc)
	r.FinalDataRole = "heldout"
	r.FinalDataReusedFrom = []string{}
	for _, old := range m.records {
		if old.Config.FinalSeed == r.Config.FinalSeed && old.FrozenIncumbent == r.FrozenIncumbent {
			r.FinalDataReusedFrom = append(r.FinalDataReusedFrom, old.ID)
		}
	}
	slices.Sort(r.FinalDataReusedFrom)
	if len(r.FinalDataReusedFrom) > 0 {
		r.FinalDataRole = "reused-development"
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := m.persist(clone(r)); err != nil {
		return Snapshot{}, err
	}
	m.notify()
	return public(m.records[r.ID]), nil
}
func (m *Manager) control(ctx context.Context, id, op string, cmd library.Command) (Snapshot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[id]
	if !ok {
		return Snapshot{}, ErrNotFound
	}
	fp := fingerprint(struct {
		Op      string
		Command library.Command
	}{op, cmd})
	if old, ok := r.Commands[cmd.CommandID]; ok {
		if old != fp {
			return Snapshot{}, library.ErrConflict
		}
		return public(r), nil
	}
	if m.failure != nil {
		return Snapshot{}, m.failure
	}
	if m.closing || !library.ValidCommandID(cmd.CommandID) || len(r.Commands) >= 1024 {
		return Snapshot{}, library.ErrInvalid
	}
	if r.Version != cmd.ExpectedVersion {
		return Snapshot{}, library.ErrConflict
	}
	switch op {
	case "stop":
		if r.State == jobs.Queued {
			r.State = jobs.Stopped
		} else if r.State == jobs.Running {
			r.State = jobs.Stopping
		} else {
			return Snapshot{}, library.ErrConflict
		}
	case "resume":
		if !r.CanResume {
			return Snapshot{}, library.ErrConflict
		}
		if r.Execution.Platform != runtime.GOOS+"/"+runtime.GOARCH || r.Execution.GoVersion != runtime.Version() {
			return Snapshot{}, fmt.Errorf("%w: resume requires the recorded platform and Go version", library.ErrInvalid)
		}
		r.State = jobs.Queued
		r.Error = ""
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	r = clone(r)
	r.Version++
	r.Revision++
	r.Commands[cmd.CommandID] = fp
	if err := m.persist(r); err != nil {
		return Snapshot{}, err
	}
	m.notify()
	return public(m.records[id]), nil
}
func (m *Manager) Stop(ctx context.Context, id string, c library.Command) (Snapshot, error) {
	return m.control(ctx, id, "stop", c)
}
func (m *Manager) Resume(ctx context.Context, id string, c library.Command) (Snapshot, error) {
	return m.control(ctx, id, "resume", c)
}
func terminal(state string) bool {
	return state == jobs.Completed || state == jobs.Failed || state == jobs.Stopped || state == jobs.Interrupted
}
func (m *Manager) Wait(ctx context.Context, id string) (Snapshot, error) {
	for {
		m.mu.Lock()
		r, ok := m.records[id]
		ch := m.changed
		failure := m.failure
		m.mu.Unlock()
		if !ok {
			return Snapshot{}, ErrNotFound
		}
		if failure != nil {
			return Snapshot{}, failure
		}
		if terminal(r.State) {
			return public(r), nil
		}
		select {
		case <-ctx.Done():
			return Snapshot{}, ctx.Err()
		case <-ch:
		}
	}
}
func (m *Manager) Close() error {
	m.mu.Lock()
	m.closing = true
	m.notify()
	m.mu.Unlock()
	<-m.done
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.failure
}
func (m *Manager) loop() {
	defer close(m.done)
	for {
		m.mu.Lock()
		if m.closing || m.failure != nil {
			for _, r := range m.records {
				if r.State == jobs.Queued || r.State == jobs.Running || r.State == jobs.Stopping {
					r.State = jobs.Interrupted
					r.Revision++
					if err := m.persist(r); err != nil {
						m.storageFailure(err)
						break
					}
				}
			}
			m.mu.Unlock()
			return
		}
		ids := []string{}
		for id, r := range m.records {
			if r.State == jobs.Queued {
				ids = append(ids, id)
			}
		}
		slices.Sort(ids)
		id := ""
		if len(ids) > 0 {
			id = ids[0]
			r := m.records[id]
			r.State = jobs.Running
			r.Revision++
			if err := m.persist(r); err != nil {
				m.storageFailure(err)
				m.mu.Unlock()
				return
			}
		}
		m.mu.Unlock()
		if id == "" {
			<-m.wake
			continue
		}
		m.run(id)
	}
}
func (m *Manager) storageFailure(err error) {
	m.failure = err
	close(m.changed)
	m.changed = make(chan struct{})
	m.notify()
}

func (m *Manager) run(id string) {
	for {
		m.mu.Lock()
		r := m.records[id]
		if m.closing || r.State == jobs.Stopping {
			r.State = jobs.Stopped
			if m.closing {
				r.State = jobs.Interrupted
			}
			r.Revision++
			if err := m.persist(r); err != nil {
				m.storageFailure(err)
			}
			m.mu.Unlock()
			return
		}
		r = clone(r)
		m.mu.Unlock()
		err := m.step(&r)
		m.mu.Lock()
		current := m.records[id]
		r.Version = current.Version
		r.Commands = current.Commands
		if current.State == jobs.Stopping && r.State == jobs.Running {
			r.State = jobs.Stopping
		}
		r.Revision = current.Revision + 1
		if err != nil {
			r.State = jobs.Failed
			r.Error = err.Error()
		}
		if e := m.persist(r); e != nil {
			m.storageFailure(e)
			m.mu.Unlock()
			return
		}
		done := terminal(r.State)
		m.mu.Unlock()
		if done {
			return
		}
	}
}
