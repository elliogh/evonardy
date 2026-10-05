// Package library publishes validated, immutable inference packages.
package library

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"evonardy/internal/agent"
	"evonardy/internal/features"
	"evonardy/internal/game"
	"evonardy/internal/storage"
)

const RandomID = "builtin/random-v1"
const HeuristicID = "builtin/heuristic-v1"
const FormatVersion = 1

var ErrInvalid = errors.New("invalid request")
var ErrNotFound = errors.New("bot not found")
var ErrConflict = errors.New("command or version conflict")

type Command struct {
	CommandID       string `json:"command_id"`
	ExpectedVersion uint64 `json:"expected_version"`
}
type Receipt struct {
	Hash    string `json:"hash"`
	Version uint64 `json:"version"`
}
type SaveRequest struct {
	Command
	SourceID string `json:"source_bot_id"`
	Name     string `json:"name"`
}
type RenameRequest struct {
	Command
	Name string `json:"name"`
}
type Inference struct {
	Exploration float64 `json:"exploration"`
	TieBreak    string  `json:"tie_break"`
}
type Manifest struct {
	ID               string    `json:"id"`
	FormatVersion    int       `json:"format_version"`
	Ruleset          string    `json:"ruleset"`
	Evaluator        string    `json:"evaluator"`
	FeaturesVersion  string    `json:"features_version"`
	Architecture     []int     `json:"architecture"`
	Inference        Inference `json:"inference"`
	ValuePerspective string    `json:"value_perspective"`
	Objective        string    `json:"objective"`
	ModelSHA256      string    `json:"model_sha256"`
	Source           string    `json:"source"`
	ParentID         string    `json:"parent_id"`
	CreatedAt        string    `json:"created_at"`
}
type Model struct {
	Weights []float64 `json:"weights"`
}
type metadata struct {
	Name     string             `json:"name"`
	Version  uint64             `json:"version"`
	Commands map[string]Receipt `json:"commands"`
}
type Card struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Kind            string `json:"kind"`
	Source          string `json:"source"`
	CreatedAt       string `json:"created_at"`
	Builtin         bool   `json:"builtin"`
	Available       bool   `json:"available"`
	Reason          string `json:"reason"`
	MetadataVersion uint64 `json:"metadata_version"`
}
type Detail struct {
	Card
	Manifest *Manifest `json:"manifest"`
}
type Library struct {
	store *storage.Store
	mu    sync.Mutex
}

func New(store *storage.Store) *Library { return &Library{store: store} }

func ValidCommandID(id string) bool {
	if len(id) < 1 || len(id) > 128 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return false
		}
	}
	return true
}

func modelID(id string) bool {
	if len(id) != 64 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
func hash(data []byte) string      { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func fingerprint(value any) string { data, _ := json.Marshal(value); return hash(data) }

func identity(m Manifest) string {
	m.ID = ""
	m.CreatedAt = ""
	m.Source = ""
	m.ParentID = ""
	data, _ := json.Marshal(m)
	return hash(data)
}

func builtin(id string) (Card, bool) {
	if id != RandomID && id != HeuristicID {
		return Card{}, false
	}
	kind, name := "random", "Random"
	if id == HeuristicID {
		kind, name = "heuristic", "Heuristic"
	}
	return Card{ID: id, Name: name, Kind: kind, Source: "Built-in baseline", Builtin: true, Available: true}, true
}

func DecodeJSON(data []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	return nil
}

func (l *Library) load(id string) (Manifest, Model, metadata, error) {
	var m Manifest
	var model Model
	var meta metadata
	if !modelID(id) {
		return m, model, meta, ErrNotFound
	}
	base := filepath.Join("bots", id)
	data, err := l.store.Read(filepath.Join(base, "manifest.json"), 65536)
	if os.IsNotExist(err) {
		return m, model, meta, ErrNotFound
	}
	if err != nil {
		return m, model, meta, err
	}
	if err := DecodeJSON(data, &m); err != nil {
		return m, model, meta, fmt.Errorf("%w: invalid manifest", ErrInvalid)
	}
	if m.ID != id || identity(m) != id || m.FormatVersion != FormatVersion || m.Ruleset != game.Ruleset || m.ValuePerspective != "white" || m.Objective != "win" || m.Inference.Exploration != 0 {
		return m, model, meta, fmt.Errorf("%w: incompatible model contract", ErrInvalid)
	}
	if _, err := time.Parse(time.RFC3339Nano, m.CreatedAt); err != nil {
		return m, model, meta, fmt.Errorf("%w: invalid model timestamp", ErrInvalid)
	}
	data, err = l.store.Read(filepath.Join(base, "model.json"), 65536)
	if err != nil {
		return m, model, meta, err
	}
	if hash(data) != m.ModelSHA256 {
		return m, model, meta, fmt.Errorf("%w: model checksum mismatch", ErrInvalid)
	}
	if err := DecodeJSON(data, &model); err != nil {
		return m, model, meta, fmt.Errorf("%w: invalid model JSON", ErrInvalid)
	}
	if err := validateModel(m, model); err != nil {
		return m, model, meta, err
	}
	data, err = l.store.Read(filepath.Join(base, "metadata.json"), 1<<20)
	if err != nil {
		return m, model, meta, err
	}
	if err := DecodeJSON(data, &meta); err != nil || meta.Version < 1 || len(meta.Commands) > 2048 || validName(meta.Name) != nil {
		return m, model, meta, fmt.Errorf("%w: invalid metadata", ErrInvalid)
	}
	return m, model, meta, nil
}

func validateModel(m Manifest, model Model) error {
	switch m.Evaluator {
	case "linear":
		if m.FeaturesVersion != features.Version || !slices.Equal(m.Architecture, []int{features.Size, 1}) || m.Inference.TieBreak != "stable_first" || len(model.Weights) != features.Size {
			return fmt.Errorf("%w: incompatible linear shapes or features", ErrInvalid)
		}
	case "random":
		if m.FeaturesVersion != "none" || len(m.Architecture) != 0 || len(model.Weights) != 0 || m.Inference.TieBreak != "uniform_unique" {
			return fmt.Errorf("%w: incompatible random model", ErrInvalid)
		}
	default:
		return fmt.Errorf("%w: unsupported evaluator", ErrInvalid)
	}
	for _, w := range model.Weights {
		if math.IsNaN(w) || math.IsInf(w, 0) || math.Abs(w) > 1e6 {
			return fmt.Errorf("%w: weights must be finite and within +/-1000000", ErrInvalid)
		}
	}
	return nil
}

func validName(name string) error {
	if strings.TrimSpace(name) == "" || len(name) > 128 {
		return fmt.Errorf("%w: name must be 1..128 bytes", ErrInvalid)
	}
	return nil
}

func (l *Library) Get(id string) (Detail, error) {
	if card, ok := builtin(id); ok {
		return Detail{Card: card}, nil
	}
	m, _, meta, err := l.load(id)
	if err != nil {
		return Detail{}, err
	}
	return Detail{Card: Card{ID: id, Name: meta.Name, Kind: m.Evaluator, Source: m.Source, CreatedAt: m.CreatedAt, Available: true, MetadataVersion: meta.Version}, Manifest: &m}, nil
}

func (l *Library) List() ([]Card, error) {
	a, _ := builtin(HeuristicID)
	b, _ := builtin(RandomID)
	cards := []Card{a, b}
	entries, err := l.store.List("bots")
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !modelID(entry.Name()) {
			continue
		}
		detail, err := l.Get(entry.Name())
		if err != nil {
			cards = append(cards, Card{ID: entry.Name(), Name: "Incompatible model " + entry.Name()[:8], Source: "Saved model", Reason: err.Error()})
		} else {
			cards = append(cards, detail.Card)
		}
	}
	return cards, nil
}

func (l *Library) Agent(id string, source agent.IntSource) (agent.Agent, error) {
	if id == RandomID {
		return agent.NewRandom(source), nil
	}
	if id == HeuristicID {
		return agent.Linear{Weights: features.DefaultWeights}, nil
	}
	m, model, _, err := l.load(id)
	if err != nil {
		return nil, err
	}
	if m.Evaluator == "random" {
		return agent.NewRandom(source), nil
	}
	var weights features.Vector
	copy(weights[:], model.Weights)
	return agent.Linear{Weights: weights}, nil
}

func (l *Library) publish(name string, model Model, kind, source, parent string) (Card, error) {
	if err := validName(name); err != nil {
		return Card{}, err
	}
	m := Manifest{FormatVersion: FormatVersion, Ruleset: game.Ruleset, Evaluator: kind, ValuePerspective: "white", Objective: "win", Inference: Inference{TieBreak: "stable_first"}, FeaturesVersion: features.Version, Architecture: []int{features.Size, 1}, Source: source, ParentID: parent, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if kind == "random" {
		m.FeaturesVersion = "none"
		m.Architecture = []int{}
		m.Inference.TieBreak = "uniform_unique"
	}
	if err := validateModel(m, model); err != nil {
		return Card{}, err
	}
	modelData, err := json.Marshal(model)
	if err != nil {
		return Card{}, err
	}
	modelData = append(modelData, '\n')
	m.ModelSHA256 = hash(modelData)
	m.ID = identity(m)
	manifestData, _ := json.Marshal(m)
	metaData, _ := json.Marshal(metadata{Name: strings.TrimSpace(name), Version: 1, Commands: map[string]Receipt{}})
	err = l.store.PublishNew(filepath.Join("bots", m.ID), map[string][]byte{"manifest.json": append(manifestData, '\n'), "model.json": modelData, "metadata.json": append(metaData, '\n')})
	if err != nil && !os.IsExist(err) {
		return Card{}, err
	}
	detail, err := l.Get(m.ID)
	return detail.Card, err
}

func (l *Library) SaveLinear(name string, weights []float64, source, parent string) (Card, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.publish(name, Model{Weights: append([]float64{}, weights...)}, "linear", source, parent)
}

func (l *Library) SaveCopy(req SaveRequest) (Card, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !ValidCommandID(req.CommandID) || req.ExpectedVersion != 0 {
		return Card{}, fmt.Errorf("%w: new snapshots require command_id and expected_version=0", ErrInvalid)
	}
	if err := validName(req.Name); err != nil {
		return Card{}, err
	}
	path := filepath.Join("bots", "commands", req.CommandID+".json")
	type saved struct {
		Hash string `json:"hash"`
		ID   string `json:"id"`
	}
	fp := fingerprint(req)
	if data, err := l.store.Read(path, 4096); err == nil {
		var receipt saved
		if err := DecodeJSON(data, &receipt); err != nil {
			return Card{}, err
		}
		if receipt.Hash != fp {
			return Card{}, ErrConflict
		}
		detail, err := l.Get(receipt.ID)
		return detail.Card, err
	} else if !os.IsNotExist(err) {
		return Card{}, err
	}
	commands, err := l.store.List(filepath.Join("bots", "commands"))
	if err != nil {
		return Card{}, err
	}
	if len(commands) >= 10000 {
		return Card{}, fmt.Errorf("%w: save command limit reached", ErrInvalid)
	}
	kind := "linear"
	model := Model{Weights: append([]float64{}, features.DefaultWeights[:]...)}
	if req.SourceID == RandomID {
		kind = "random"
		model.Weights = []float64{}
	} else if req.SourceID != HeuristicID {
		m, loaded, _, err := l.load(req.SourceID)
		if err != nil {
			return Card{}, err
		}
		kind, model = m.Evaluator, loaded
	}
	card, err := l.publish(req.Name, model, kind, "Saved baseline snapshot", req.SourceID)
	if err != nil {
		return Card{}, err
	}
	data, _ := json.Marshal(saved{Hash: fp, ID: card.ID})
	if err := l.store.WriteNew(path, append(data, '\n')); err != nil {
		return Card{}, err
	}
	return card, nil
}

func (l *Library) Rename(id string, req RenameRequest) (Card, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !ValidCommandID(req.CommandID) {
		return Card{}, fmt.Errorf("%w: invalid command_id", ErrInvalid)
	}
	if _, ok := builtin(id); ok {
		return Card{}, fmt.Errorf("%w: built-in names cannot be changed", ErrInvalid)
	}
	_, _, meta, err := l.load(id)
	if err != nil {
		return Card{}, err
	}
	fp := fingerprint(req)
	if receipt, ok := meta.Commands[req.CommandID]; ok {
		if receipt.Hash != fp {
			return Card{}, ErrConflict
		}
		detail, err := l.Get(id)
		return detail.Card, err
	}
	if meta.Version != req.ExpectedVersion {
		return Card{}, ErrConflict
	}
	if err := validName(req.Name); err != nil {
		return Card{}, err
	}
	if len(meta.Commands) >= 2048 {
		return Card{}, fmt.Errorf("%w: metadata command limit reached", ErrInvalid)
	}
	if meta.Commands == nil {
		meta.Commands = map[string]Receipt{}
	}
	meta.Name = strings.TrimSpace(req.Name)
	meta.Version++
	meta.Commands[req.CommandID] = Receipt{Hash: fp, Version: meta.Version}
	data, _ := json.Marshal(meta)
	if err := l.store.WriteAtomic(filepath.Join("bots", id, "metadata.json"), append(data, '\n')); err != nil {
		return Card{}, err
	}
	detail, err := l.Get(id)
	return detail.Card, err
}
