# EvoNardy

[![CI](https://github.com/elliogh/evonardy/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/elliogh/evonardy/actions/workflows/ci.yml)

A local open-source **long nardy (long backgammon)** project with a Go backend
and a React/TypeScript UI. M0–M6 implements the game engine, Random/Heuristic
agents, verifiable replays, browser play, a persistent bot library, and real
GA-linear, GA-MLP, neural TD(0)/TD(lambda), and synchronous Hybrid training with checkpoint/resume and independent evaluation. Research mode adds auditable
multi-seed comparisons, paired uncertainty and separate held-out confirmation.

The UI and all repository content are maintained in English.

## Getting started

The backend requires Go 1.25+. Use Node 22.12+ LTS or Node 24+ and npm for the frontend.
Development and storage verification currently support macOS and Linux. Writing
on platforms without flock is explicitly rejected. External APIs and
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

Open **Training** and select GA-linear, GA-MLP, TD(0), TD(lambda), or Hybrid. Use the smoke preset
for a short run, then save an evaluated population candidate or a neural TD game checkpoint to My bots.
Neural runs expose learning/exploration settings, actual TD updates, and measured
TD error; Hybrid separates training from selection and shows participant lineage. These are diagnostics, not claims of playing strength.
The **Training games** board automatically plays sampled actual matches from the
run. Pause, step through turns, change playback speed, or follow newer games.
Training continues at full speed while you watch. The latest sample survives
restart; runs created before this feature have no recorded preview until resumed.
**Evaluate** runs separate paired games against frozen opponents; the library
shows the latest completed result. Stop saves a checkpoint and Resume continues
it after restart. You can play a saved bot while another run trains.

The shared offline lifecycle is also available (stop the server first):

```bash
bin/evonardy train --config configs/ga-linear-smoke.json --data-dir ./data --save-name "My GA bot"
bin/evonardy train --algorithm td0 --config configs/td-zero-smoke.json --data-dir ./data --save-name "My TD bot"
bin/evonardy train --algorithm td-lambda --config configs/td-lambda-smoke.json --data-dir ./data --save-name "My trace bot"
bin/evonardy train --algorithm ga-mlp --config configs/ga-mlp-smoke.json --data-dir ./data --save-name "My neural GA bot"
bin/evonardy train --algorithm hybrid --config configs/hybrid-smoke.json --data-dir ./data --save-name "My Hybrid bot"
bin/evonardy resume --run <run-id> --data-dir ./data
bin/evonardy evaluate --bot <saved-bot-id> --config configs/evaluation-smoke.json --data-dir ./data
```

SIGINT/SIGTERM checkpoints an active job after its current bounded batch (one learner game for TD/Hybrid). See
[docs/TRAINING.md](docs/TRAINING.md) for fitness, counters, limits, and reproducibility.

Run bounded baseline simulations independently:

```bash
go run ./cmd/evonardy simulate --config configs/baseline-smoke.json --data-dir ./data
go run ./cmd/evonardy replay --dir ./data/replays
```

Game and worker counts are bounded by the configuration. Flags `--seed`, `--games`,
`--workers`, `--max-turns`, `--white`, and `--black` override JSON values. Without
JSON, defaults are 4 games, seed 42, 2 workers, 1200 turns, and Heuristic against
Random. Offline simulations currently accept only `random` and `heuristic`. Invalid parameters
are rejected before simulation. Ctrl+C cancels the simulation batch. Durable resume is available for training
and evaluation jobs.

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

To refine an existing linear bot, choose **Training → GA-linear → Start from saved
bot → ev1**, then set the new run's budget. The source stays immutable; a saved
child is a candidate until independently evaluated. See the
[saved-bot workflow](docs/TRAINING.md#start-ga-linear-from-a-saved-bot) and
`configs/ga-linear-refine.json` for an explicit 30720-game example.

## Rules and checks

GitHub Actions runs the full validation suite on every pull request to `main`,
every push to `main`, and manual dispatches using Ubuntu 24.04, Go 1.25, and
Node.js 24. `main` requires an up-to-date PR and a successful **CI result**.
Logs, informational benchmarks, and Chromium reports are retained for seven days.
See [the CI workflow](docs/WORKFLOW.md#required-ci) for failure investigation.

The `long-nardy-fnr2026-nocube-v1` profile has no hitting, doubling cube, or
triple-win backgammon result. Formal rules and the bearing-off clarification are in
[docs/RULES.md](docs/RULES.md). Heuristic features and weights are in
[docs/FEATURES.md](docs/FEATURES.md).

The shared M4 neural input is specified in [docs/ENCODER.md](docs/ENCODER.md):
a versioned 56-value position vector. [docs/NEURAL.md](docs/NEURAL.md) specifies
the shared `56 → 32 → 1` tanh network and its verified parameter gradients.
Frozen neural models can be published/loaded through the Go library and played
without a learner. [docs/TD.md](docs/TD.md) specifies sequential TD(0)/TD(lambda)
learning and real self-play. Neural CLI/API jobs support safe stop, deterministic
resume, frozen saves, and independent evaluation. The browser uses the same
methods and lifecycle. The bounded neural presets run four games each.

```bash
make test    # gofmt check, vet, Go tests + race, frontend typecheck/test/build
make smoke   # baselines/replays + bounded training, frozen saves and evaluation
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
M3 checks additionally train real coefficients, save/reload a candidate, run an
independent evaluation, stop/resume after restart, and finish a game against the
frozen bot while another run trains.
M4 adds real TD(0) and TD(lambda) browser flows through train, frozen save, server
restart, reload, independent evaluation, sampled self-play, stop/resume, and a
complete game against the saved neural model. Watching or playing leaves learner
streams and published weights unchanged. M5 adds two population scenarios: archived
GA-MLP/Hybrid candidate saves, lineage and replacement inspection, restart, independent
evaluation, stopped/resumed learning, and immutable neural play. The suite contains
ten real browser scenarios. Saved-source GA-linear adds exact source copying,
three-opponent budgets, immutable descendant lineage and restart checks. M6 adds a real five-seed, three-method experiment,
checks every plotted budget against the server, downloads and hashes the immutable
report, and verifies the selected candidate and final verdict after restart.

## Research mode

Open **Research** in the browser. Review the full configuration, including five
independent training seeds, method budgets, frozen opponents, development/final
seeds and the predeclared confirmation criterion. **Use research smoke preset**
runs a small two-seed experiment; its final batch deliberately cannot confirm a
champion. Edit the full JSON for larger experiments, then choose **Start experiment**.
Stop/resume preserves the committed games and selection lock.

The results show every seed against measured decisions and trainer/selection time,
all-seed method intervals, paired method differences and separate final evidence.
Each model is saved in **My bots** for independent evaluation and play. Download the
immutable JSON report to audit exact configs, model manifests, raw schedules,
results, measured counters and version/platform/build metadata. A reused final
schedule is labeled development data and cannot supply independent confirmation.

```bash
go run ./cmd/evonardy experiment --config configs/research-smoke.json \
  --name "Bounded comparison" --data-dir .cache/research
# Resume the saved configuration after stopping; use the ID from the JSON output.
go run ./cmd/evonardy experiment-resume --experiment EXPERIMENT_ID \
  --data-dir .cache/research
```

`make smoke` includes this bounded lifecycle; long training remains outside CI.
The default is a starting protocol, not a recommendation about sufficient playing
strength. Read [research contracts](docs/RESEARCH.md) before interpreting intervals,
comparing compute budgets or confirming a champion.

### Bounded local strength study

The [October 2026 study](docs/STRENGTH_STUDY.md) retained ev1 after 24 training
runs and a complete seven-policy league; the best new GA-linear model remains
a separate candidate.

The separate offline study compares all five implemented training methods,
including GA-linear, against a saved incumbent. It uses five training seeds,
4096 physical training/selection games per method/seed, development evaluations,
a finalist round robin and one independent 500-pair confirmation. Its two-hour
deadline includes persistence; no new training starts after 105 minutes.

```bash
go build -o bin/evonardy-study ./cmd/evonardy-study
bin/evonardy-study --print-config
bin/evonardy-study --data-dir local-artifacts/study \
  --incumbent-dir /path/to/archived/bots/MODEL_ID
# SIGINT checkpoints the current job. Resume keeps the original deadline.
bin/evonardy-study --data-dir local-artifacts/study --resume
# Stop the application before installing the selected immutable packages.
bin/evonardy-study --data-dir local-artifacts/study --resume --install-to ./data
```

The incumbent package is validated and copied without changing its parameters.
Reports are `study/report.md` and `study/report.json` inside the isolated directory;
JSON includes all raw evaluation scores and actual work, including interrupted
jobs. Incomplete seed cohorts have no aggregate method estimate. The tournament
can select a candidate from completed runs, but only completed independent
confirmation can replace the incumbent. Installation retains the incumbent when
confirmation fails and exposes the best newly trained candidate separately.
The existing browser Research protocol remains available unchanged.

Keep backups in `local-artifacts`, which is excluded from Git and ordinary CI.
For a complete local reset, first stop every writer, archive the entire data
directory together with checksums, then create a fresh directory. Moving only bot
folders leaves dangling references in training, evaluation and game records.

Tasks and current status: [GitHub Issues](https://github.com/elliogh/evonardy/issues)
and the [EvoNardy roadmap](https://github.com/users/elliogh/projects/1).
Stages and acceptance criteria: [milestones](https://github.com/elliogh/evonardy/milestones).
Contribution workflow: [docs/WORKFLOW.md](docs/WORKFLOW.md).
Historical verification results: [docs/PROGRESS.md](docs/PROGRESS.md).
API and persistence contracts: [docs/API.md](docs/API.md).
Training and reproducibility: [docs/TRAINING.md](docs/TRAINING.md).
The full implementation brief is in `evonardy-codex-plan/EVONARDY_PLAN.md`.

## License

MIT. The implementation brief is maintained in English. FNR source documents
are not included in the distribution; RULES contains the project's formal
specification and a link to the primary source.
