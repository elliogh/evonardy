# EvoNardy

A local open-source **long nardy (long backgammon)** project with a Go backend
and a React/TypeScript UI. M0–M2 implements the game engine, Random/Heuristic
agents, CLI simulations, verifiable replays, browser play, and a persistent bot
library. Training and evaluation are planned for M3+; snapshots currently save
actual baseline parameters and are explicitly marked as not evaluated.

The UI and all repository content are maintained in English.

## Getting started

The backend requires Go 1.25+. Use Node 22.12+ LTS or Node 24+ and npm for the frontend.
Development and storage verification currently support macOS and Linux. Writing
on platforms without flock is explicitly rejected. Python, external APIs, and
a database are not required.

```bash
make setup
make dev
```

Open `http://127.0.0.1:5173`. `make dev` runs both the Go API on port 8080 and
Vite on port 5173, proxies API requests, and stops both processes on Ctrl+C.
Choose White or Black, select a bot, roll when prompted, and select highlighted
checkers and destinations. If two dice permit the same destination, choose a die.
Undo or reset a draft, then confirm the complete turn. The server saves dice and
drafts, so refreshing or restarting does not reroll them. Recent games resume
from their saved state. A completed game has a downloadable, verifiable replay.

For a production frontend with the local server:

```bash
make build
bin/evonardy serve --addr 127.0.0.1:8080 --data-dir ./data --web-dir web/dist
```

Open `http://127.0.0.1:8080`. Keep `web/dist` next to the project or pass its path;
embedding the assets in the binary is planned for M7. The server rejects external
listen addresses. Data defaults to `./data`; `EVONARDY_DATA_DIR` overrides it for
`make dev`. One process owns a data directory at a time, so stop the server before
running an offline CLI command against the same directory.

The library has built-in Random and Heuristic cards. Save a snapshot to keep its
inference package, and rename saved cards without changing their model identity.
Identical parameters and inference contracts share one snapshot. No training or
evaluation results are fabricated. CLI library commands are also available:

```bash
bin/evonardy bots save --source builtin/heuristic-v1 --name "My baseline" --data-dir ./data
bin/evonardy bots list --data-dir ./data
```

Run bounded baseline simulations independently:

```bash
go run ./cmd/evonardy simulate --config configs/baseline-smoke.json --data-dir ./data
go run ./cmd/evonardy replay --dir ./data/replays
```

Game and worker counts are bounded by the configuration. Flags `--seed`, `--games`,
`--workers`, `--max-turns`, `--white`, and `--black` override JSON values. Without
JSON, defaults are 4 games, seed 42, 2 workers, 1200 turns, and Heuristic against
Random. Offline simulations currently accept only `random` and `heuristic`. Invalid parameters
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

## Rules and checks

The `long-nardy-fnr2026-nocube-v1` profile has no hitting, doubling cube, or
triple-win backgammon result. Formal rules and the bearing-off clarification are in
[docs/RULES.md](docs/RULES.md). Heuristic features and weights are in
[docs/FEATURES.md](docs/FEATURES.md).

```bash
make test    # gofmt check, vet, Go tests + race, frontend typecheck/test/build
make smoke   # 12 complete baseline games + 2 truncated games; verify all replays
make bench   # move generation, evaluation, simulation; measured time and allocations
cd web && npx playwright install chromium
cd .. && make e2e  # real local server; temporary data; full browser game and restart
```

An additional bounded fuzz run:

```bash
go test ./internal/game -run '^$' -fuzz FuzzLegalTurnInvariants -fuzztime=5s -parallel=2
```

Position fixtures run for both colors. Generator completeness is compared with
an independent slow enumerator rather than only the engine's own ApplyTurn.
Tests also cover symmetry, checker conservation, input immutability, replay
corruption, reproducibility across worker counts, locking, immutable publication,
model save/load, concurrent command retries, session recovery, and HTTP/SSE.
Browser checks exercise the library, keyboard moves, a full game against Heuristic,
refresh, server restart, draft undo, replay download, and a 390px viewport.

Status and actual results: [docs/PROGRESS.md](docs/PROGRESS.md).
API and persistence contracts: [docs/API.md](docs/API.md).
Next milestone: M3, with GA-linear training, checkpoint/resume, and evaluation.
The full implementation brief is in `evonardy-codex-plan/EVONARDY_PLAN.md`.

## License

MIT. The implementation brief is maintained in English. FNR source documents
are not included in the distribution; RULES contains the project's formal
specification and a link to the primary source.
