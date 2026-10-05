# EvoNardy

A local open-source **long nardy (long backgammon)** project with a Go backend
and a React/TypeScript UI. The first iteration, M0–M1, implements the game engine,
Random/Heuristic agents, CLI simulations, and verifiable replays. The frontend
is a scaffold. Human play, the model library, training, and the server are not
available yet; `serve/train/resume` commands are not implemented.

The UI and all repository content are maintained in English.

## Getting started

The backend requires Go 1.25+. The frontend requires Node 20.19+ or 22.12+ and npm.
Development and storage verification currently support macOS and Linux. Writing
on platforms without flock is explicitly rejected. Python, external APIs, and
a database are not required.

```bash
go run ./cmd/evonardy --help
go run ./cmd/evonardy simulate --config configs/baseline-smoke.json --data-dir ./data
go run ./cmd/evonardy replay --dir ./data/replays
```

Game and worker counts are bounded by the configuration. Flags `--seed`, `--games`,
`--workers`, `--max-turns`, `--white`, and `--black` override JSON values. Without
JSON, defaults are 4 games, seed 42, 2 workers, 1200 turns, and Heuristic against
Random. Only `random` and `heuristic` bots are available. Invalid parameters
are rejected before simulation. Ctrl+C cancels the batch; resume is planned for M3.

Simulations publish `replays/<run-id>/game-NNNN.json` and
`runs/<run-id>/summary.json`. Run IDs are unique and existing files are never
overwritten. An OS lock protects the data directory from a second independent
writer and is released when a process crashes. Cancellation during publication
can leave verified replays without a summary; such a directory is not a completed run.

Summaries contain actual wins, mars wins, the mean signed White score, and
simulation wall-clock time excluding file publication. `truncated` games are
reported separately and excluded from win rates. Result metrics are null when
no games finish. These smoke statistics do not establish playing strength or
provide a research fitness evaluation.

Verify one replay and inspect its final position:

```bash
go run ./cmd/evonardy replay --file ./data/replays/<run-id>/game-0000.json
```

Replays store actual dice and full actions, including the opening roll.
Verification does not require bots or repeat historical move selection.

Frontend development:

```bash
make setup
make dev
```

Open `http://127.0.0.1:5173`. The page describes the current project stage.
`make build` creates the CLI at `bin/evonardy` and a separate frontend bundle
at `web/dist`. UI embedding and server-backed play will arrive in later milestones;
the current CLI does not serve a browser game.

## Rules and checks

The `long-nardy-fnr2026-nocube-v1` profile has no hitting, doubling cube, or
triple-win backgammon result. Formal rules and the bearing-off clarification are in
[docs/RULES.md](docs/RULES.md). Heuristic features and weights are in
[docs/FEATURES.md](docs/FEATURES.md).

```bash
make test    # gofmt check, vet, Go tests + race, frontend typecheck/test/build
make smoke   # 12 complete baseline games + 2 truncated games; verify all replays
make bench   # move generation, evaluation, simulation; measured time and allocations
```

An additional bounded fuzz run:

```bash
go test ./internal/game -run '^$' -fuzz FuzzLegalTurnInvariants -fuzztime=5s -parallel=2
```

Position fixtures run for both colors. Generator completeness is compared with
an independent slow enumerator rather than only the engine's own ApplyTurn.
Tests also cover symmetry, checker conservation, input immutability, replay
corruption, reproducibility across worker counts, locking, and immutable file publication.

Status and actual results: [docs/PROGRESS.md](docs/PROGRESS.md).
Next milestone: M2, with HTTP/SSE, human play on an SVG board, sessions, and a library.
The full implementation brief is in `evonardy-codex-plan/EVONARDY_PLAN.md`.

## License

MIT. The implementation brief is maintained in English. FNR source documents
are not included in the distribution; RULES contains the project's formal
specification and a link to the primary source.
