package study

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"time"

	"evonardy/internal/agent"
	"evonardy/internal/encoder"
	"evonardy/internal/features"
	"evonardy/internal/game"
	"evonardy/internal/jobs"
	"evonardy/internal/library"
	"evonardy/internal/neural"
	"evonardy/internal/research"
	"evonardy/internal/storage"
	"evonardy/internal/training"
)

type Run struct {
	Method         string            `json:"method"`
	Seed           uint64            `json:"seed"`
	JobID          string            `json:"job_id"`
	BotID          string            `json:"bot_id"`
	Manifest       *library.Manifest `json:"manifest"`
	Counters       training.Counters `json:"counters"`
	TrainerSeconds float64           `json:"trainer_seconds"`
	DevelopmentID  string            `json:"development_job_id"`
	RandomID       string            `json:"random_job_id"`
	Estimate       research.Estimate `json:"estimate"`
}

type Match struct {
	A        string            `json:"a"`
	B        string            `json:"b"`
	JobID    string            `json:"job_id"`
	Estimate research.Estimate `json:"estimate"`
}

type Standing struct {
	BotID  string `json:"bot_id"`
	Games  int    `json:"games"`
	Wins   int    `json:"wins"`
	Points int    `json:"points"`
}

type Evidence struct {
	JobID      string                 `json:"job_id"`
	Evaluation jobs.EvaluationSummary `json:"evaluation"`
}

type Work struct {
	JobID       string            `json:"job_id"`
	Kind        string            `json:"kind"`
	State       string            `json:"state"`
	Counters    training.Counters `json:"counters"`
	WallSeconds float64           `json:"wall_seconds"`
}

type State struct {
	Version         string                  `json:"version"`
	ID              string                  `json:"id"`
	Config          Config                  `json:"config"`
	StartedAt       time.Time               `json:"started_at"`
	UpdatedAt       time.Time               `json:"updated_at"`
	Platform        string                  `json:"platform"`
	GoVersion       string                  `json:"go_version"`
	Revision        string                  `json:"source_revision"`
	Modified        bool                    `json:"source_modified"`
	Ruleset         string                  `json:"ruleset"`
	Features        string                  `json:"features_version"`
	Encoder         string                  `json:"encoder_version"`
	Network         string                  `json:"network_version"`
	Schedule        string                  `json:"schedule"`
	IncumbentID     string                  `json:"incumbent_id"`
	IncumbentSHA    string                  `json:"incumbent_sha256"`
	Phase           string                  `json:"phase"`
	Status          string                  `json:"status"`
	Reason          string                  `json:"reason"`
	Runs            []Run                   `json:"runs"`
	Methods         []research.MethodResult `json:"methods"`
	Differences     []research.Difference   `json:"differences"`
	Matches         []Match                 `json:"matches"`
	Standings       []Standing              `json:"standings"`
	BestNewID       string                  `json:"best_new_id"`
	CandidateID     string                  `json:"candidate_id"`
	SelectionLocked bool                    `json:"selection_locked"`
	ConfirmationID  string                  `json:"confirmation_job_id"`
	Verdict         *research.Verdict       `json:"verdict"`
	SelectedID      string                  `json:"selected_id"`
	Evidence        []Evidence              `json:"evaluation_evidence"`
	Work            []Work                  `json:"work"`
}

type runner struct {
	store *storage.Store
	bots  *library.Library
	jobs  *jobs.Manager
	s     State
	out   io.Writer
}

func digest(data []byte) string        { x := sha256.Sum256(data); return hex.EncodeToString(x[:]) }
func policyHash(p agent.Policy) string { b, _ := json.Marshal(p); return digest(b) }

// Execute owns an isolated study directory. A saved protocol cannot be replaced.
// The original absolute deadline remains in force across process restarts.
func Execute(ctx context.Context, store *storage.Store, incumbent string, cfg Config, resume bool, out io.Writer) (State, error) {
	if err := cfg.Validate(); err != nil {
		return State{}, err
	}
	r := runner{store: store, bots: library.New(store), out: out}
	data, err := store.Read("study/checkpoint.json", 16<<20)
	if err == nil {
		if !resume {
			return State{}, fmt.Errorf("study already exists; use --resume")
		}
		var envelope struct {
			SHA256 string          `json:"sha256"`
			State  json.RawMessage `json:"state"`
		}
		if err = library.DecodeJSON(data, &envelope); err != nil {
			return State{}, err
		}
		if digest(envelope.State) != envelope.SHA256 {
			return State{}, fmt.Errorf("study checksum mismatch")
		}
		if err = library.DecodeJSON(envelope.State, &r.s); err != nil {
			return State{}, err
		}
		if r.s.Version != Version || r.s.Ruleset != game.Ruleset || r.s.Encoder != encoder.Version || r.s.Network != neural.Version || r.s.Features != features.Version || r.s.Schedule != "paired-evaluation-v1" || r.s.Platform != runtime.GOOS+"/"+runtime.GOARCH || r.s.GoVersion != runtime.Version() {
			return State{}, fmt.Errorf("study version or execution environment changed")
		}
		if err = r.s.Config.Validate(); err != nil {
			return State{}, err
		}
		if incumbent != "" && incumbent != r.s.IncumbentID {
			return State{}, fmt.Errorf("incumbent changed")
		}
		cfg = r.s.Config
	} else if os.IsNotExist(err) {
		if resume {
			return State{}, fmt.Errorf("no study to resume")
		}
		for _, dir := range []string{"runs", "evaluations", "experiments", "games"} {
			entries, e := store.List(dir)
			if e != nil {
				return State{}, e
			}
			if len(entries) > 0 {
				return State{}, fmt.Errorf("new study requires an isolated directory without existing %s", dir)
			}
		}
		var token [16]byte
		if _, err = rand.Read(token[:]); err != nil {
			return State{}, err
		}
		p, e := r.bots.Freeze(incumbent)
		if e != nil {
			return State{}, e
		}
		if incumbent == library.HeuristicID || incumbent == library.RandomID {
			return State{}, fmt.Errorf("incumbent must be a saved model")
		}
		r.s = State{Version: Version, ID: hex.EncodeToString(token[:]), Config: cfg, StartedAt: time.Now().UTC(), Platform: runtime.GOOS + "/" + runtime.GOARCH, GoVersion: runtime.Version(), IncumbentID: incumbent, IncumbentSHA: policyHash(p), Phase: "training", Status: "running", SelectedID: incumbent, Runs: []Run{}, Matches: []Match{}}
		r.s.Ruleset, r.s.Features, r.s.Encoder, r.s.Network, r.s.Schedule = game.Ruleset, features.Version, encoder.Version, neural.Version, "paired-evaluation-v1"
		if info, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range info.Settings {
				if setting.Key == "vcs.revision" {
					r.s.Revision = setting.Value
				}
				if setting.Key == "vcs.modified" {
					r.s.Modified = setting.Value == "true"
				}
			}
		}
		if r.s.Revision == "" {
			r.s.Revision = "unknown"
		}
		if err = r.persist(); err != nil {
			return r.s, err
		}
	} else {
		return State{}, err
	}
	p, err := r.bots.Freeze(r.s.IncumbentID)
	if err != nil {
		return r.s, err
	}
	if policyHash(p) != r.s.IncumbentSHA {
		return r.s, fmt.Errorf("frozen incumbent changed")
	}
	for _, run := range r.s.Runs {
		detail, e := r.bots.Get(run.BotID)
		if e != nil {
			return r.s, e
		}
		if run.Manifest == nil || detail.Manifest == nil || detail.Manifest.ModelSHA256 != run.Manifest.ModelSHA256 {
			return r.s, fmt.Errorf("published study model changed")
		}
	}
	if r.s.Status == "completed" {
		return r.s, r.report()
	}
	r.s.Status, r.s.Reason = "running", ""
	if err = r.persist(); err != nil {
		return r.s, err
	}
	deadline := r.s.StartedAt.Add(cfg.duration())
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	r.jobs, err = jobs.New(store, r.bots)
	if err != nil {
		return r.s, err
	}
	err = r.execute(ctx)
	closeErr := r.jobs.Close()
	r.s.Work = nil
	for _, kind := range []string{jobs.Training, jobs.Evaluation} {
		xs, e := r.jobs.List(kind)
		if e != nil {
			err = errors.Join(err, e)
			continue
		}
		for _, x := range xs {
			r.s.Work = append(r.s.Work, Work{x.ID, x.Kind, x.State, x.Counters, x.WallSeconds})
			if kind == jobs.Evaluation {
				full, getErr := r.jobs.Get(x.ID)
				if getErr != nil {
					err = errors.Join(err, getErr)
					continue
				}
				r.recordEvidence(full)
			}
		}
	}
	if closeErr != nil {
		err = errors.Join(err, closeErr)
	}
	if err != nil {
		r.s.Status = "failed"
		r.s.Reason = err.Error()
		if errors.Is(err, context.Canceled) {
			r.s.Status = "interrupted"
		}
		if errors.Is(err, context.DeadlineExceeded) {
			r.s.Status = "time_limit"
		}
	}
	if e := r.persist(); e != nil {
		return r.s, errors.Join(err, e)
	}
	if e := r.report(); e != nil {
		return r.s, errors.Join(err, e)
	}
	return r.s, err
}

func (r *runner) persist() error {
	r.s.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(r.s)
	if err != nil {
		return err
	}
	envelope := struct {
		SHA256 string          `json:"sha256"`
		State  json.RawMessage `json:"state"`
	}{digest(data), data}
	b, err := json.Marshal(envelope)
	if err != nil {
		return err
	}
	return r.store.WriteAtomic("study/checkpoint.json", append(b, '\n'))
}

func (r *runner) command(key string) library.Command {
	return library.Command{CommandID: digest([]byte(r.s.ID + "/" + key))[:32]}
}

func (r *runner) seed(label string) uint64 {
	x := sha256.Sum256([]byte(r.s.ID + "/" + label))
	var seed uint64
	for _, b := range x[:8] {
		seed = seed<<8 | uint64(b)
	}
	return seed
}

func (r *runner) wait(ctx context.Context, x jobs.Snapshot) (jobs.Snapshot, error) {
	if x.CanResume {
		cmd := r.command(fmt.Sprintf("resume/%s/%d", x.ID, x.Version))
		cmd.ExpectedVersion = x.Version
		var err error
		x, err = r.jobs.Resume(ctx, x.ID, cmd)
		if err != nil {
			return x, err
		}
	}
	x, err := r.jobs.Wait(ctx, x.ID)
	if err != nil {
		return x, err
	}
	if x.State != jobs.Completed {
		return x, fmt.Errorf("job %s %s: %s", x.ID, x.State, x.Error)
	}
	return x, nil
}

func (r *runner) evaluate(ctx context.Context, key, bot string, opponents []string, pairs int, seed uint64) (jobs.Snapshot, error) {
	x, err := r.jobs.Evaluate(ctx, jobs.EvaluateRequest{Command: r.command(key), BotID: bot, Config: jobs.EvaluationConfig{Seed: seed, Pairs: pairs, Workers: r.s.Config.Workers, MaxTurns: r.s.Config.MaxTurns, OpponentIDs: opponents}})
	if err != nil {
		return x, err
	}
	x, err = r.wait(ctx, x)
	if x.Evaluation != nil {
		r.recordEvidence(x)
		if e := r.persist(); e != nil {
			return x, errors.Join(err, e)
		}
	}
	return x, err
}

func (r *runner) recordEvidence(x jobs.Snapshot) {
	if x.Evaluation == nil {
		return
	}
	for i, e := range r.s.Evidence {
		if e.JobID == x.ID {
			r.s.Evidence[i] = Evidence{x.ID, *x.Evaluation}
			return
		}
	}
	r.s.Evidence = append(r.s.Evidence, Evidence{x.ID, *x.Evaluation})
}

func (r *runner) execute(ctx context.Context) error {
	c := r.s.Config
	if r.s.Phase == "training" {
		cutoff := r.s.StartedAt.Add(time.Duration(c.TrainingCutoffSeconds) * time.Second)
		for si, seed := range c.Seeds {
			for offset := range c.Methods {
				mi := (si + offset) % len(c.Methods)
				method := c.Methods[mi]
				found := false
				for _, run := range r.s.Runs {
					if run.Method == method.Name && run.Seed == seed {
						found = true
						break
					}
				}
				if found {
					continue
				}
				key := fmt.Sprintf("training/%s/%d", method.Name, seed)
				if time.Now().After(cutoff) {
					if _, e := r.jobs.Get(r.command(key).CommandID); errors.Is(e, jobs.ErrNotFound) {
						continue
					}
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				fmt.Fprintf(r.out, "Training %s seed %d (%d/%d seeds)\n", method.Name, seed, si+1, len(c.Seeds))
				req := expanded(method, seed)
				req.Command = r.command(key)
				x, err := r.jobs.Start(ctx, req)
				if err != nil {
					return err
				}
				x, err = r.wait(ctx, x)
				if err != nil {
					return err
				}
				card, err := r.save(ctx, x, method.Name, seed)
				if err != nil {
					return err
				}
				detail, err := r.bots.Get(card.ID)
				if err != nil {
					return err
				}
				dev, err := r.evaluate(ctx, key+"/development", card.ID, []string{library.HeuristicID, r.s.IncumbentID}, c.DevelopmentPairs, r.seed(fmt.Sprintf("development/%d", si)))
				if err != nil {
					return err
				}
				e, err := research.Analyze([][]training.Score{dev.Evaluation.Scores}, c.Bootstrap)
				if err != nil {
					return err
				}
				random, err := r.evaluate(ctx, key+"/random", card.ID, []string{library.RandomID}, c.RandomPairs, r.seed(fmt.Sprintf("random/%d", si)))
				if err != nil {
					return err
				}
				r.s.Runs = append(r.s.Runs, Run{Method: method.Name, Seed: seed, JobID: x.ID, BotID: card.ID, Manifest: detail.Manifest, Counters: x.Counters, TrainerSeconds: x.WallSeconds, DevelopmentID: dev.ID, RandomID: random.ID, Estimate: e})
				fmt.Fprintf(r.out, "Completed %s seed %d: development %.1f%% [%0.1f, %0.1f], training games %d\n", method.Name, seed, 100*e.WinRate, 100*e.WinInterval.Low, 100*e.WinInterval.High, x.Counters.Games)
				if err = r.persist(); err != nil {
					return err
				}
			}
		}
		if err := r.methodEstimates(); err != nil {
			return err
		}
		r.s.Phase = "tournament"
		if err := r.persist(); err != nil {
			return err
		}
	}
	if r.s.Phase == "tournament" {
		finalists := []string{r.s.IncumbentID, library.HeuristicID}
		bestNew := -1
		for _, method := range c.Methods {
			best := -1
			for i, run := range r.s.Runs {
				if run.Method == method.Name && (best < 0 || betterRun(run, r.s.Runs[best])) {
					best = i
				}
			}
			if best >= 0 {
				id := r.s.Runs[best].BotID
				duplicate := false
				for _, old := range finalists {
					if old == id {
						duplicate = true
					}
				}
				if !duplicate {
					finalists = append(finalists, id)
				}
				if bestNew < 0 || betterRun(r.s.Runs[best], r.s.Runs[bestNew]) {
					bestNew = best
				}
			}
		}
		if bestNew >= 0 {
			r.s.BestNewID = r.s.Runs[bestNew].BotID
		}
		r.s.Standings = make([]Standing, len(finalists))
		for i, id := range finalists {
			r.s.Standings[i].BotID = id
		}
		r.s.Matches = []Match{}
		for i := range finalists {
			for j := i + 1; j < len(finalists); j++ {
				fmt.Fprintf(r.out, "Tournament match %d vs %d\n", i, j)
				x, err := r.evaluate(ctx, fmt.Sprintf("tournament/%d/%d", i, j), finalists[i], []string{finalists[j]}, c.TournamentPairs, r.seed("tournament"))
				if err != nil {
					return err
				}
				e, err := research.Analyze([][]training.Score{x.Evaluation.Scores}, c.Bootstrap)
				if err != nil {
					return err
				}
				r.s.Matches = append(r.s.Matches, Match{A: finalists[i], B: finalists[j], JobID: x.ID, Estimate: e})
				addMatch(&r.s.Standings[i], &r.s.Standings[j], x.Evaluation.Scores)
				if err = r.persist(); err != nil {
					return err
				}
			}
		}
		leader := 0
		newLeader := -1
		for i, s := range r.s.Standings {
			if s.Games > 0 && s.Wins*r.s.Standings[leader].Games > r.s.Standings[leader].Wins*s.Games {
				leader = i
			}
			if i >= 2 && (newLeader < 0 || s.Wins*r.s.Standings[newLeader].Games > r.s.Standings[newLeader].Wins*s.Games) {
				newLeader = i
			}
		}
		if newLeader >= 0 {
			r.s.BestNewID = finalists[newLeader]
		}
		r.s.CandidateID = finalists[leader]
		r.s.SelectionLocked = true
		r.s.Phase = "confirmation"
		if err := r.persist(); err != nil {
			return err
		}
	}
	if r.s.Phase == "confirmation" {
		if !r.s.SelectionLocked || r.s.CandidateID == "" {
			return fmt.Errorf("candidate must be locked before confirmation")
		}
		if r.s.CandidateID == r.s.IncumbentID {
			r.s.Reason = "Incumbent won the development tournament; no replacement tested"
		} else {
			fmt.Fprintf(r.out, "Locked candidate %s; independent confirmation against %s\n", r.s.CandidateID, r.s.IncumbentID)
			x, err := r.evaluate(ctx, "confirmation", r.s.CandidateID, []string{r.s.IncumbentID}, c.Confirmation.Pairs, r.seed("confirmation"))
			if err != nil {
				return err
			}
			v, err := research.Confirm(x.Evaluation.Scores, c.Confirmation)
			if err != nil {
				return err
			}
			r.s.ConfirmationID = x.ID
			r.s.Verdict = &v
			r.s.Reason = v.Reason
			if v.Status == "confirmed" {
				r.s.SelectedID = r.s.CandidateID
			}
		}
		r.s.Phase = "completed"
		r.s.Status = "completed"
	}
	return nil
}

func betterRun(a, b Run) bool {
	return a.Estimate.WinRate > b.Estimate.WinRate || (a.Estimate.WinRate == b.Estimate.WinRate && a.Seed < b.Seed)
}

func addMatch(a, b *Standing, scores []training.Score) {
	for _, s := range scores {
		a.Games++
		b.Games++
		points := s.Outcome.Points
		if s.Outcome.Winner == s.Side {
			a.Wins++
			a.Points += points
			b.Points -= points
		} else {
			b.Wins++
			b.Points += points
			a.Points -= points
		}
	}
}

func (r *runner) save(ctx context.Context, x jobs.Snapshot, method string, seed uint64) (library.Card, error) {
	if len(x.Saved) > 0 {
		returnCard, err := r.bots.Get(x.Saved[0].BotID)
		return returnCard.Card, err
	}
	var candidate string
	if x.NeuralCandidate != nil {
		candidate = x.NeuralCandidate.ID
	} else if len(x.NeuralCandidates) > 0 {
		candidate = x.NeuralCandidates[0].ID
	} else if len(x.Candidates) > 0 {
		candidate = x.Candidates[0].ID
	} else {
		return library.Card{}, fmt.Errorf("no completed candidate")
	}
	cmd := r.command("save/" + x.ID)
	cmd.ExpectedVersion = x.Version
	return r.jobs.Save(ctx, x.ID, jobs.SaveRequest{Command: cmd, Generation: x.Generation, CandidateID: candidate, Name: fmt.Sprintf("Study %s seed %d", method, seed)})
}

func (r *runner) methodEstimates() error {
	r.s.Methods = []research.MethodResult{}
	r.s.Differences = []research.Difference{}
	groups := [][][]training.Score{}
	for _, method := range r.s.Config.Methods {
		cohort := [][]training.Score{}
		for _, seed := range r.s.Config.Seeds {
			for _, run := range r.s.Runs {
				if run.Method == method.Name && run.Seed == seed {
					x, err := r.jobs.Get(run.DevelopmentID)
					if err != nil {
						return err
					}
					cohort = append(cohort, x.Evaluation.Scores)
				}
			}
		}
		if len(cohort) != len(r.s.Config.Seeds) {
			continue
		}
		e, err := research.Analyze(cohort, r.s.Config.Bootstrap)
		if err != nil {
			return err
		}
		r.s.Methods = append(r.s.Methods, research.MethodResult{Algorithm: method.Name, Estimate: e})
		groups = append(groups, cohort)
	}
	for i := range groups {
		for j := i + 1; j < len(groups); j++ {
			e, err := research.Compare(groups[i], groups[j], r.s.Config.Bootstrap)
			if err != nil {
				return err
			}
			r.s.Differences = append(r.s.Differences, research.Difference{A: r.s.Methods[i].Algorithm, B: r.s.Methods[j].Algorithm, Estimate: e})
		}
	}
	return nil
}

func (r *runner) report() error {
	data, err := json.MarshalIndent(r.s, "", "  ")
	if err != nil {
		return err
	}
	if err = r.store.WriteAtomic("study/report.json", append(data, '\n')); err != nil {
		return err
	}
	return r.store.WriteAtomic("study/report.md", []byte(markdown(r.s)))
}
