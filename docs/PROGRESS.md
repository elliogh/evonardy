# EvoNardy progress

## M0–M1 iteration

- [x] Reviewed the original brief and available tools; no existing code was present.
- [x] Created a separate local Git repository; the parent repository was the home directory.
- [x] Downloaded the FNR PDF dated September 14, 2026 and reviewed articles 19, 20, 27, and 28.
- [x] M0: RULES, ARCHITECTURE, Go/React scaffold, configurations, and lockfile.
- [x] M1: position tests, coordinates, state, and full-turn generation.
- [x] M1: Random/Heuristic, bounded simulations, replay, and CLI.
- [x] Acceptance: gofmt, vet, unit/property/oracle tests, race, frontend checks, and smoke.

**M0 and M1 are complete.** The checks below describe that iteration. M2 is
complete as recorded in its own section below; the next milestone is M3.

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
