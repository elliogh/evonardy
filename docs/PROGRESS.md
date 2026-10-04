# EvoNardy progress

## M0–M1 iteration

- [x] Reviewed the original brief and available tools; no existing code was present.
- [x] Created a separate local Git repository; the parent repository was the home directory.
- [x] Downloaded the FNR PDF dated September 14, 2026 and reviewed articles 19, 20, 27, and 28.
- [x] M0: RULES, ARCHITECTURE, Go/React scaffold, configurations, and lockfile.
- [x] M1: position tests, coordinates, state, and full-turn generation.
- [x] M1: Random/Heuristic, bounded simulations, replay, and CLI.
- [x] Acceptance: gofmt, vet, unit/property/oracle tests, race, frontend checks, and smoke.

**M0 and M1 are complete.** Checks were run against the final implementation
of this iteration. The next milestone is M2; M2–M7 are not implemented yet.

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

## Iteration limitations

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

## Next iteration

M2: server game sessions, HTTP/SSE, SVG board, command idempotency,
session recovery, and an immutable model library.
