# EvoNardy progress

## M0–M1 iteration

- [x] Reviewed the original brief and available tools; no existing code was present.
- [x] Created a separate local Git repository; the parent repository was the home directory.
- [x] Downloaded the FNR PDF dated September 14, 2026 and reviewed articles 19, 20, 27, and 28.
- [x] M0: RULES, ARCHITECTURE, Go/React scaffold, configurations, and lockfile.
- [x] M1: position tests, coordinates, state, and full-turn generation.
- [x] M1: Random/Heuristic, bounded simulations, replay, and CLI.
- [x] Acceptance: gofmt, vet, unit/property/oracle tests, race, frontend checks, and smoke.

**M0 through M3 are complete.** The checks below describe their respective
iterations; the next milestone is M4.

Public test boundaries from the brief: ValidatePosition, LegalTurns, ApplyTurn,
Result, LegalContinuations, agent action selection, simulation, and replay.
A game reaching its turn limit stops the generation's fitness evaluation:
truncation gets no reward and cannot be used to select a generation. Training is M3+.

## Checks and environment

Go 1.25.5 darwin/amd64; Node 23.11.0; npm 10.9.2. Python/pypdf was used only
to read the primary source and is not a project dependency.

Completed checks:

- Position fixtures for both colors, symmetry, and independent oracle: pass.
- Random/Heuristic, PRNG state restore, replay corruption, and storage locking: pass.
- Replay contents from simulations with workers=1/2 match bitwise: pass.
- Frontend typecheck, Vitest (1 test), and production build: pass.
- Installing the latest Vitest range triggered an npm 10.9.2 resolver error.
  Compatible versions were pinned: React 19.2.3, Vite 7.3.1, Vitest 4.0.18,
  TypeScript 5.9.3, and plugin-react 5.1.2; installation and build succeeded.
- `make test`: gofmt check, go vet, go test, and go test -race for all eight
  Go packages; frontend typecheck, Vitest, and Vite build: pass.
- `make smoke`: 12 completed, 0 truncated baseline games, and 1118 full turns.
  A separate max_turns=1 batch produced 2 truncated games with outcome=null
  and result metrics=null. All 14 saved replays reproduced and matched hashes.
  Heuristic won 11 of 12 against Random; 7 games ended in mars. This small fixed
  sample does not establish playing strength or algorithm superiority.
- Bounded fuzz run (`-fuzztime=5s -parallel=2`): 5655 executions without errors.
- `make build`: Go CLI and frontend production bundle: pass.
- Go CLI also built for linux/amd64; Linux execution was not tested.
- `bin/evonardy --help` runs on macOS.

## Benchmarks

`make bench`, darwin/amd64, Go 1.25.5, Intel Core i5-1038NG7 @ 2.00GHz,
8 logical CPUs. One short measurement, not a promise for other hardware:

| Check | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| LegalTurns, documented midgame fixture, 3–3 | 250161 | 272315 | 467 |
| HeuristicScore, opening position | 219.2 | 0 | 0 |
| Simulation, seed 42, 1 Heuristic–Random game | 15281868 | 14369968 | 31395 |

Benchmark fixtures are next to the game/features/arena tests; measurements
apply to those inputs. CPU time was not measured and wall-clock time was not
multiplied by worker count.

## M0–M1 limitations at completion

The frontend is a scaffold. HTTP/SSE, human play, the model library, and training
are not implemented or presented as working. The UI is not embedded in the Go CLI.
Storage uses Unix flock; unsupported platforms receive an explicit error.
Simulation resume is unavailable. Saved replays are independent of the trainer.

## English-language positioning

The user requested an English product and English repository content, while
conversation remains in Russian. UI copy, HTML language/title, README, technical
documentation, and implementation briefs have been translated. The earlier
Russian UI/documentation requirement has been replaced in the brief. The language
instruction was removed from AGENTS.md and its suggested template at the user's
request. Game rules, identifiers, and formats are unchanged. Frontend typecheck,
Vitest, and production build passed. A scan
of repository source files found no remaining Cyrillic text; the HTML locale is `en`.

## M2 iteration — complete

- [x] Atomic mutable session/metadata storage and immutable model publication.
- [x] Built-in cards, saved linear/random snapshots, compatibility checks, and rename.
- [x] Authoritative sessions, persistent dice/drafts, idempotent versioned commands.
- [x] Local HTTP/SSE server, bounded requests, Host/Origin checks, and serve CLI.
- [x] English browser UI: bot library, SVG board, legal moves, undo, confirmation, history.
- [x] Acceptance: full human–Heuristic game, refresh/restart recovery, repeated commands,
  backend tests/race, frontend tests/build, and browser end-to-end checks.

Public test boundaries are model save/load and metadata, session commands and
recovery, HTTP/SSE behavior, and the real browser flow. These are the M2 boundaries
specified in the implementation brief. Training remains M3; M2 saves actual
baseline parameters and does not label them as trained or evaluated models.

### Implemented behavior

The server owns opening dice, later rolls, full-turn validation, bot inference,
drafts, versions, and durable command receipts. Confirmation includes the next
bot turn in one transaction. Saved snapshots preserve model identity and choices;
renames update metadata only. Failed validation/cancellation before commit do not
persist changes. Finished histories reproduce without inference. The frontend
uses engine-validated previews and continuations, supports keyboard selection,
explicit ambiguous-die choices, undo/reset, history, and recent-game resume.
The game ruleset and inference tie behavior are unchanged.

`serve`, `bots list`, and `bots save` are available. `make dev` now starts the
backend and Vite proxy; `make build` produces the CLI and separate frontend assets.
Protocol, schemas, compatibility, persistence, and resource limits are documented
in `docs/API.md`. AGENTS.md points to progress without caching a milestone number;
it contains no language policy.

### Actual M2 checks

Final suite on Go 1.25.5 darwin/amd64, Node 23.11.0, npm 10.9.2:

- `make test`: gofmt, vet, Go tests and race across all 11 packages, frontend
  typecheck, 2 Vitest tests, and production build: pass.
- Session tests complete human games against Heuristic for both White and Black,
  validate replay/final hashes, recover after reopen, preserve dice and drafts,
  reject stale/reused commands, handle concurrent retries exactly once, respect
  cancellation, and prevent mutation through returned snapshots: pass.
- Library tests preserve linear and random decisions, immutable identities and
  models after rename/new publication; reject bad shapes/nonfinite weights and
  corrupted checksums; keep unavailable cards visible: pass.
- HTTP tests cover strict command fields, client-assigned dice rejection, stale
  versions, body limits, Host/Origin rejection, metadata, and real-socket SSE: pass.
- `make smoke`: this batch completed 12 baseline games with 1118 turns and produced
  2 separately truncated games with null result metrics. All 42 accumulated smoke
  replays (36 completed, 6 truncated) verified. Fixed-seed outcomes match M1.
- `make build`: frontend assets and macOS CLI: pass.
- `make e2e`: 2 real Chromium scenarios passed in 40.7 seconds. The full game uses
  browser controls against Heuristic; the scenario saves/renames a snapshot,
  refreshes, restarts the actual server, restores and undoes a draft, loses an
  already-committed confirmation response and retries it once, finishes the game,
  and downloads its replay. The second scenario plays a saved model as Black and
  checks that another browser receives draft/turn updates over SSE.
- Desktop and 390×844 screenshots were inspected; the narrow view has no horizontal
  page overflow. Checker selection uses keyboard focus/Enter in browser tests.
- Development launcher: library GET and versioned game POST through Vite with
  its browser Origin succeed; shutdown stops both local servers: pass.
- Updated Vite to 7.3.6 and Vitest to 4.1.11 after npm audit advisories. npm 10's
  resolver crashed during the upgrade; a temporary npm 11.6.0 completed it without
  changing global npm. Compatible transitive fixes and a clean npm 10 `ci` passed;
  audit reported 0 vulnerabilities. Playwright is pinned to 1.57.0 in the lockfile.
- Node 23 is outside Vitest's declared engine range and produces an npm warning;
  the checks above passed on it. README recommends supported LTS versions.
- Repository source/documentation scan found no Cyrillic text; `git diff --check`
  passed. No long training or publication was run.

`make bench`, same machine and 200ms samples; these are measurements, not promises:

| Check | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| LegalTurns, midgame fixture, 3–3 | 289227 | 272312 | 467 |
| HeuristicScore, opening position | 255.9 | 0 | 0 |
| Simulation, seed 42, 1 Heuristic–Random game | 16558833 | 14370787 | 31406 |

### Remaining work

M3 is next: real GA-linear training, bounded jobs, checkpoint/resume, manual
candidate saves, and independent evaluation. No training/evaluation API or fake
statistics are exposed in M2. Timeline playback, model import/export, neural
inference, and release embedding remain later milestones. Linux execution and
non-Chromium browsers were not tested in M2; storage still requires Unix flock.

## M3 iteration — complete

- [x] Frozen-policy arena matches with paired dice and measured work.
- [x] GA-linear generations, balanced development games, independent random streams.
- [x] Atomic checkpoints, bounded job queue, stop/resume, interruption recovery.
- [x] Immutable candidate publication and independent frozen-model evaluation.
- [x] Shared CLI/API lifecycle, SSE, training/evaluation UI, library results.
- [x] Deterministic continuous/resumed runs, model integrity, full browser acceptance.
- [x] Required test/smoke/build/bench checks and actual results.

Public boundaries from M3 in the brief: generation training and fitness,
checkpoint/resume, candidate save/load, independent evaluation, HTTP job commands,
and real browser train → save → play. A truncated selection game fails the run;
an incomplete generation never selects parents. Only bounded verification runs
will be started by the implementation agent.

### Implemented M3 behavior

The pure GA evaluates whole paired games against fixed Heuristic/Random policies,
then applies elitism, tournament selection, uniform crossover, and Gaussian
mutation to the nine existing linear weights. Keyed random streams keep variation,
dice, and agent choices independent. One durable job runs at a time with up to
eight match workers. Batch checkpoints retain partial-generation work and support
explicit stop/resume and crash recovery. Versioned generation archives allow
manual publication during training without changing any saved model.

`train`, `resume`, and `evaluate` share the server's job manager. HTTP exposes the
same controls; small SSE revision notices are coalesced twice per second. The UI
adds Training, saved-generation charts, candidate selection/publication, Evaluate,
and links to actual independent results in My bots. User control versions are
separate from background progress revisions. No game rules or inference ties
changed. Protocol and reproducibility are documented in API.md and TRAINING.md.
AGENTS.md was not changed.

### Actual M3 checks

Go 1.25.5 darwin/amd64, Node 23.11.0, npm 10.9.2, Chromium 143:

- `make test`: gofmt, vet, Go tests and race across 13 packages; frontend typecheck,
  4 Vitest tests, and production build: pass. The final additional early-publication
  check also passed normal/race job tests. Frontend checks were repeated after
  presentation refinements.
- Completed matches validate their real replays and measured inference work.
  Paired games preserve opening/per-side dice when policies or sides change.
- GA tests match continuous vs serialized-prefix resume and workers=1/2 for actual
  coefficients, histories, and counters. Incomplete/truncated generations never
  receive fitness or select parents.
- Job tests stop/reopen/resume, reconstruct an abrupt crash from an actual running
  checkpoint, match the uninterrupted run, reject checksum corruption, publish
  during an active run, and preserve that early model through subsequent training.
  Loaded weights equal the chosen candidate; inference works after manager close.
- Independent evaluation uses separate seed domains, freezes caller-owned config,
  preserves published weights, matches workers=1/2, and rejects truncated aggregate
  results. HTTP tests cover creation retry, stale versions, candidate save,
  generation retrieval, kind isolation, evaluation, and small real-socket SSE.
- `make smoke`: 12 baseline games completed in 1118 turns; 2 separate games
  truncated at one turn. All 56 accumulated replays (48 completed, 8 truncated)
  verified. The GA smoke completed 32 games across 2 generations, 2809 decisions,
  61995 linear evaluations, 27 coefficient mutations, and 3 child crossovers.
  The best candidate remained the initial baseline on this tiny schedule; other
  coefficients evolved, and the browser published a candidate with changed weights.
  The CLI saved/reloaded its selected candidate and completed 8 independent games:
  6 wins, 4 mars wins, mean signed points 1.0. These small samples do not establish
  playing strength or superiority of GA.
- `make build`: frontend and macOS CLI: pass (also run by `make e2e`).
- `make e2e`: all 3 actual Chromium scenarios passed in 1.4 minutes. M2 scenarios
  still pass. M3 loses an already-committed training-create response and retries
  the identical command, trains 32 games, publishes changed coefficients, completes
  an independent 4-game batch, starts another 96-game run, opens a game against the
  frozen model while training, stops/restarts/resumes the run, and finishes a real
  browser game. Manifest and session snapshots survive restart unchanged.
- Desktop and 390×844 training screenshots inspected; chart labels remain readable
  and the narrow view has no horizontal page overflow.
- Repository source/documentation contains no Cyrillic; `git diff --check`: pass.
  No external service, long training, release publication, or dependency upgrade
  was used. Prettier ran from a temporary pinned package; no dependency was added.

`make bench`, same Intel Core i5-1038NG7 machine and 200ms samples, after the main
suites finished; measurements, not promises:

| Check | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| LegalTurns, midgame fixture, 3–3 | 243328 | 272308 | 467 |
| HeuristicScore, opening position | 235.0 | 0 | 0 |
| Simulation, seed 42, 1 Heuristic–Random game | 14600203 | 14370384 | 31408 |

### Remaining work after M3

M4 is next: the shared encoder/MLP and RL with TD(0), then TD(lambda). GA-MLP and
Hybrid remain M5; paired statistical intervals and comparisons remain M6; release
embedding and packaging remain M7. M3 has no automatic champion promotion or
confidence intervals. Baseline simulation batches remain nonresumable; training
and evaluation jobs are resumable. Archive cleanup is manual, storage requires
Unix flock, and Linux execution/non-Chromium browsers remain untested.

## Training game viewer — complete

The user requested visible games during training. The UI shows actual sampled training
matches on the Training screen with automatic playback, pause, turn navigation,
dice, participants, and results. Playback is independent of the training budget
and scheduler; slow or disconnected viewers never block matches. Retain only the
latest sampled replay per run, and restore it from checkpoints. Older checkpoints
without a replay remain loadable. This replaces the brief's separate exhibition
watch-game suggestion for this feature at the user's request.

Checks use the already established public boundaries: training wave results and recorded
matches, durable job snapshots/recovery, HTTP job routes, and actual browser flow.
They verify sampled records against the training scores, unchanged deterministic work,
checkpoint compatibility, authoritative Go positions, and playback while training.

### Viewer behavior and checks

One actual replay is selected from each completed worker batch. Sampling consumes
no random draws and adds no matches. Job snapshots include a small notice; the
new GET watch route returns that replay and positions reconstructed by Go. The
checkpoint holds only the latest replay under its existing checksum envelope.
Loading validates replay legality, hashes, schedule, participants, and notice;
format-1 checkpoints with no viewer data remain compatible. Published inference
packages, ruleset, and generation fitness are unchanged.

Training games autoplay on the run page, with candidate/opponent colors, dice,
moves, a terminal result, previous/next turn, a slider, playback speed, and a
Follow training switch. Show latest game switches immediately. The browser keeps
only the playing sample and the latest available one, and coalesces fetches to
one in flight. Slow/hidden viewers do not pause training. Existing completed runs
without a recorded replay show an honest empty preview; new or resumed runs
record samples. Documentation and the brief were updated to match the request.

Actual checks on the same Go 1.25.5 darwin/amd64 and Node/npm/Chromium environment:

- `make test`: gofmt, vet, all 13 Go packages, race, frontend typecheck, 4 Vitest
  tests, and build: pass. Additional focused training/job tests and race also pass.
- Wave records match their actual evaluated score, dice seed, participants, side,
  turn count, and outcome; replay validation succeeds. Viewing leaves counters,
  versions, and revisions unchanged. Returned board/replay mutations cannot change
  durable records. Reopen restores the same replay and Go positions. A format-1
  checkpoint with watcher fields absent loads without inventing an old game.
- HTTP checks the watch route, missing-game 404, Go positions, and existing SSE.
- `make smoke`: 12 complete baseline games, 2 separately truncated games, and all
  70 accumulated replays (60 complete, 10 truncated) verified. Training again
  produced exactly 32 games, 2809 decisions, 61995 forward evaluations, 27 mutations,
  and 3 crossovers; the independent batch completed 8 games. Work matches M3 before
  observation was added. No long training or release publication was run.
- `make build`: frontend and macOS CLI: pass.
- `make e2e`: all 4 actual Chromium scenarios passed in 1.4 minutes. The viewer
  scenario confirms autoplay advances, compares every board point with the actual
  Go position after a turn, pauses while job counters continue increasing, steps
  forward/backward, seeks to the true winner, switches samples, and restores the
  same replay after server restart. The previous 3 play/train/evaluate scenarios
  still pass.
- After mobile label and cursor refinements, the focused viewer scenario passed
  again in 7.2 seconds including setup. Desktop and 390×844 screenshots inspected:
  controls fit, board counts/point labels remain visible, and there is no horizontal
  overflow. Final frontend/CLI build passed.
- `git diff --check`, gofmt, and the English-source scan passed. AGENTS.md and
  dependencies were unchanged. Playback cursors remain transient; retaining all
  historical training replays is outside this feature. M4 remains next.

`make bench` passed after the other suites finished, on the same Intel Core
i5-1038NG7 machine with 200ms samples; measurements, not promises:

| Check | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| LegalTurns, midgame fixture, 3–3 | 237218 | 272303 | 467 |
| HeuristicScore, opening position | 215.4 | 0 | 0 |
| Simulation, seed 42, 1 Heuristic–Random game | 14649727 | 14370574 | 31411 |
