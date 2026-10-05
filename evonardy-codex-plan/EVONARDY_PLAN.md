# EvoNardy — implementation brief for Codex

## 0. Goal and workflow

Build a local open-source **long nardy (long backgammon)** application where users train bots, save selected versions to a library, and play against them in a browser. Then add reproducible comparisons of evolutionary search, reinforcement learning, and hybrid training.

The working name is EvoNardy; its uniqueness has not been checked. Use Go for the backend, game engine, simulations, inference, and small models. Use React + TypeScript + Vite for the UI.

Main scenario:

```text
Start application → train agent → evaluate candidate → save bot
→ stop training → restart application → select bot → play as a human.
```

This is an implementation brief, not a description of existing behavior. All numerical configurations below are initial experimental settings, not promises of performance or playing strength.

Work through milestones sequentially. Before changes, inspect the repository, existing AGENTS.md, and available tools. Preserve other contributors' changes and instructions. Track tasks, priorities, acceptance criteria, and current status in GitHub Issues, milestones, and the EvoNardy roadmap; see `docs/WORKFLOW.md`. Record actual checks and incomplete work in linked issues or pull requests. `docs/PROGRESS.md` retains the pre-migration history. Run the relevant checks after each milestone.

**First working version — M0–M3:** correct long nardy, UI, bot library, GA-linear training, and human play against saved models. **Research version — M4–M7:** RL, GA-MLP, Hybrid, comparisons, and extended analysis.

Do not replace algorithms with stubs. Temporary mock data is allowed only in isolated UI tests. Hide unfinished application features or explicitly mark them unavailable. Do not launch hours of training without separate authorization; bounded smoke runs are sufficient for implementation checks.

## 1. Product boundaries

The application runs locally and listens only on `127.0.0.1` by default. The finished release embeds the UI in the Go binary; Node.js is needed for developer builds, not for release users. Use `go:embed` for embedded files [R3] and Vite for React/TypeScript builds [R4].

The first version has no accounts, money, betting, online matchmaking, microservices, Kubernetes, Kafka, external database, or cloud infrastructure. It has no LLM, paid AI API, GPU dependency, or Python runtime service. Do not build a general ML framework or a universal platform for board games.

The product UI and **all repository content** must be in English: documentation, implementation plans, identifiers, code, comments, tests, and user instructions. Communicate with the user in Russian. This policy supersedes the earlier request for Russian UI and documentation.

## 2. Fixed long-nardy profile

Use `long-nardy-fnr2026-nocube-v1`. This is a local game based on the FNR rules dated September 14, 2026, **without a doubling cube, tournament clocks, or tournament procedures**. Do not claim full implementation of official competition regulations. This profile is the project's chosen variant, not an assertion about the user's own playing rules. Keep it distinct from house variants.

Normative foundation [R1, articles 19, 20, 27, 28]:

- Each player has 15 checkers on opposite heads. Both move in the same direction; there is no hitting, bar for hit checkers, or triple-win backgammon result.
- Any opposing checker closes a point, regardless of stack size. Friendly checkers may be stacked.
- Opening: each player rolls one die and ties are rerolled. The higher result starts and uses both rolled dice without another roll.
- Usually only one checker may leave the head. On the second player's first turn, doubles 3–3, 4–4, and 6–6 allow two. Obstructions do not revoke this exception, but all landings must remain legal.
- A six-point block may not imprison all 15 opposing checkers, even during an intermediate step.
- Use the maximum number of dice. When both different dice cannot be used together, use the larger available die. Doubles provide four steps.
- Bearing off begins when all remaining checkers are home. Victory is immediate; an ordinary win scores 1 point and a mars win scores 2.

Explicitly formalize bearing off: exact bearing off is legal; an overshooting roll removes only a farthest remaining checker. Moving within home instead of bearing off is allowed if dice-use obligations are satisfied. Include dedicated examples of this clarification in `docs/RULES.md`.

Create `docs/RULES.md` with the profile, source, differences from the complete regulations, coordinates, formal conditions, and test examples. A PDF link alone is insufficient. Describe any contradiction explicitly rather than silently substituting standard-backgammon rules. Game-behavior changes require a new profile version and saved-data compatibility checks.

## 3. Coordinates and state

Suggested internal coordinates: global points `0..23`, increasing counterclockwise on the UI. Heads: `head[White]=0`, `head[Black]=12`.

```text
progress(player, globalPoint) = (globalPoint - head[player] + 24) % 24
home: progress 18..23
borne off: progress 24, stored separately
physicalPoint(player, progress) = (head[player] + progress) % 24
```

Compute new progress first, then check route exit and bearing-off conditions, and only then convert to a physical point. Applying `%24` must never turn bearing off into a return to the head.

A position stores both players' checkers, off counts, side to move, starting player, each player's first-turn completion flag, and ruleset ID. Current dice and the human move draft belong to the game session. The random source is not an agent-accessible position field.

The engine operates on positions at full-turn boundaries. Intermediate search has a separate context: remaining dice, checkers taken from the head, and step sequence. Copying state must not introduce shared mutable memory.

Six-point block formalization: inspect blocking runs along the **opponent's finite route**, not arbitrary comparisons of global indices. A run of at least 6 is prohibited when all 15 opposing checkers remain before it. A borne-off checker is not imprisoned. Never join the route's end to its beginning. Add dedicated position fixtures, including runs crossing global zero.

## 4. Turn generation and application

An agent's primary action is a **full turn for a roll**, not an individual checker step. A double can produce four steps. A pass is an explicit full action with no steps and is legal only when no other action exists.

Use DFS over possible steps and dice orders. Check landing, head limit, home conditions, and blocks after each step; checking only the final position is insufficient. Moving one checker by the sum of dice also requires a legal intermediate landing.

After enumerating full sequences, apply global obligations: first maximize the number of dice used, then apply the larger-die rule when only one of two different dice can be used. Do not greedily choose the first available die and lose legal continuations. Finishing the game takes priority; do not require nonexistent steps after the last checker bears off.

For agents, merge equivalent final states using a stable representative and deterministic sorting. For the UI, retain all legal paths or a prefix tree. Final-state deduplication must not exclude a legal alternative step order for a human. RandomAgent samples unique final states uniformly rather than duplicate path entries.

Provide `ValidatePosition`, `LegalTurns`, `ApplyTurn`, `Result`, and `LegalContinuations`. Names and signatures may be adapted while retaining their behavior. Public `ApplyTurn` validates the action; internal enumeration may use a private function for applying an already validated step.

## 5. Architecture without unnecessary infrastructure

```text
evonardy/
  cmd/evonardy/
  internal/
    game/           # rules, coordinates, state, generation
    agent/          # inference interfaces and baseline agents
    features/       # long-nardy features
    neural/         # compact MLP, gradients, serialization
    training/       # TD, GA, Hybrid, shared lifecycle
    arena/          # simulations, evaluation scheduling, metrics
    library/        # immutable bots and loading
    replay/         # game recording and playback
    app/            # game scenarios and job manager
    httpapi/        # HTTP API, SSE
    storage/        # atomic files and data-directory locking
  web/              # React, TypeScript, Vite
  configs/          # smoke and experimental configurations
  testdata/         # positions, replays, small models
  docs/
```

This is a responsibility map, not a requirement to create dozens of empty files up front. `game` has no dependencies on HTTP, UI, the filesystem, or training. CLI and server share a training workflow rather than implementing divergent versions.

Use a `data-dir` containing `bots/`, `runs/`, `replays/`, `evaluations/`, and `games/`. JSON/JSONL and model files suffice for the first version. Write through a temporary file and publish atomically, using fsync where possible. Prevent simultaneous independent writers: offline CLI training and a running server must not own the same data directory at once. UI training and play may coexist within one server.

## 6. Agents and features

An agent receives the public position, rolled dice, and legal actions. It returns its choice and optional scores for alternatives. It receives neither future rolls nor environment PRNG state nor private information about the opponent beyond the public board.

Implement `RandomAgent`, `HeuristicAgent`, a trained linear evaluator, and a neural evaluator. `Human` is a session mode, not a blocking agent inside a training worker.

Initial heuristic features: remaining-distance difference, checkers on the head and at home, off counts, occupied points, block lengths, checker distribution, rearmost-checker lag, and movement availability. Specify exact calculations and normalization for every feature. Expensive mobility evaluation across all rolls must be optional and its cost accounted for.

**Do not add blot, hitting, bar, or standard-backgammon anchor penalties.** A single checker cannot be hit here. Features are hypotheses rather than guaranteed strategy. Pip count must not be the only meaningful feature.

Exploration randomness is separate from dice randomness. Library bots disable exploration. Score ties follow stable action ordering. Select an immediate victory using the exact result rather than an incidental model output.

## 7. Saved bots and training checkpoints

A playable bot is an immutable inference package:

```text
bots/<bot-id>/
  manifest.json
  model.json
  metadata.json
```

The manifest includes format version, ruleset ID, evaluator type, encoder/features version, architecture, inference parameters, value perspective, objective, model checksum, training source, and parent version. Display metadata such as name, notes, and tags is separate from the immutable model. Evaluation results are stored separately and refer to bot IDs.

Start small models with validated JSON and float64 parameters. Specify schema, matrix shapes, weight counts, and size limits explicitly. Reject NaN/Inf, corrupted weights, and incompatible versions. A binary format may be added later under a separate version.

Bot identity must include the model, rules, and inference configuration. Renaming does not change strategy. The bot must choose the same actions before and after saving across a test-position corpus. New training never overwrites a published bot.

Trainer checkpoints additionally contain the population, hyperparameters, counters, lineage, PRNG state, schedule, and required learner state. Save at game/iteration boundaries so unfinished eligibility traces need not be serialized. Choose an explicit TD optimizer for resume, initially ordinary SGD.

Support periodic saves, manual **Save candidate**, and separate publication of a confirmed local champion. Do not equate the latest version with the best. Playing against a bot does not train it, and user statistics do not change weights.

After MVP, support export/import through a custom `.nardbot` manifest/model archive without executable code. Validate paths and reject `..`, absolute paths, symlinks, overwrites, and excessive decompressed sizes. Display incompatible bots with an explanation instead of loading them permissively.

## 8. UI and HTTP contract

Home screen: **My bots**, with cards showing agent type, source, date, measured results, and **Play**, **Evaluate**, and **Rename** actions. Built-in Random/Heuristic are available immediately and labeled accurately. Do not invent champions or ratings.

**Game** screen: SVG board, checker stack counts, direction indicator, heads and homes, dice, turn, history, draft-step undo, and full-turn confirmation. Click interaction is sufficient; drag-and-drop comes later. Undo within an unconfirmed turn is allowed in ordinary play. Undo after confirmation belongs only to a separate training mode and does not affect ordinary statistics.

The UI requests legal continuations from the server. If the die used for a step is ambiguous, offer a choice or retain the set of legal prefixes. Do not silently choose an interpretation that deprives the user of a legal continuation. Enable confirmation only for a complete legal action.

The server owns the authoritative session and turn phase. Every mutating request includes `command_id` and `expected_version`. Repeating a command never rerolls dice; a stale version receives a conflict. Clients cannot assign dice, change the opponent's turn, or substitute bot IDs midgame. Fixed-dice debug scenarios are test-only.

Example API namespace:

```text
GET    /api/bots
GET    /api/bots/{id}
PATCH  /api/bots/{id}/metadata
POST   /api/games
GET    /api/games/{id}
POST   /api/games/{id}/roll
POST   /api/games/{id}/continuations
POST   /api/games/{id}/turn
GET    /api/games/{id}/events
POST   /api/training/runs
GET    /api/training/runs/{id}
POST   /api/training/runs/{id}/stop
POST   /api/training/runs/{id}/resume
POST   /api/training/runs/{id}/save-bot
GET    /api/training/runs/{id}/events
POST   /api/evaluations
GET    /api/evaluations/{id}
GET    /api/replays/{id}
```

Session initialization records the opening contest separately. The winner already has opening dice; ordinary `/roll` must not reroll them. Document request/error schemas and synchronize them with TypeScript types.

HTTP carries commands and snapshots; SSE carries updates. Start with small events: `game.updated`, `run.progress`, `checkpoint.saved`, `run.completed`, and `run.failed`. On reconnect, requesting the authoritative snapshot is sufficient; do not promise unlimited SSE history. Bound queues so disconnected or slow browsers do not block computation. Aggregate progress, for example twice per second, instead of emitting every training turn.

**Training** screen: available method, seed, budget, worker count, start, and stop with checkpoint. Charts use only saved metrics. At the user's request, the board plays actual sampled training matches. Playback has its own pause, speed, and turn navigation; it never blocks training or changes its games. Keep observation bounded to the latest sampled replay per run and fetch full histories separately from small progress notifications.

**Comparison** appears after evaluation is implemented, with a matchup matrix and shared-position analysis by several models. Linear-evaluator and neural-value scales are not directly comparable. Do not label neural output as a calibrated win probability.

Technical security: validate Host/Origin for the local API, avoid broad CORS, bound requests, use same-origin commands, timeouts, and correct context cancellation. Do not expose an unauthenticated server on an external interface by default.

## 9. Algorithms: practical MVP and fair comparison

### 9.1 GA-linear — the first trainable bot

The genome consists of normalized-feature weights. Initial configuration: population 64, approximately 10% elitism, tournament selection, uniform crossover of named coefficients, and Gaussian mutation with configurable strength.

Fitness is the mean result of evaluation games, not a sum of good moves. The MVP uses one objective: win the game, reward +1/-1; mars points are an additional metric. Evaluate candidates against the same fixed development opponent set with balanced sides. Intrapopulation games may supplement evaluation but must not be its only basis.

Compute results after the complete generation evaluation. Worker response order must not change selection. Use separate PRNG streams for selection, crossover, mutation, and the environment. A selected candidate can be saved before the entire experiment ends.

### 9.2 Shared network for GA-MLP, RL, and Hybrid

All three methods in the main comparison use the **same encoder, MLP, and move-selection policy**. GA-linear remains a separate baseline. Comparing a linear genome with a more expressive network without disclosing that difference does not support conclusions about learning algorithms.

Initial encoder: 56 numbers:

```text
24 White checker counts / 15
24 Black checker counts / 15
 2 borne-off counts / 15
 2 side-to-move one-hot values
 2 first-turn-not-yet-completed flags
 2 starting-player one-hot values
```

Global-point ordering is fixed. Input is the full position **before the next roll**, including states after completed actions. Current/future dice are unnecessary in this encoder: known dice have already generated candidates and future dice are hidden. The same board with a different side to move is a different state.

MLP: `56 → 32 → 1`, tanh hidden and output layers, float64, explicit shapes, and compact, verifiable backpropagation. This is an initial architecture, not a claim of sufficient playing strength. Correctness comes before profiling.

### 9.3 RL: TD(0), then TD(lambda)

The methodological source is TD learning and self-play. Historical TD-Gammon used **standard backgammon**, not long nardy. Transfer the learning idea, not its rules, pretrained weights, or promised level of play [R2].

Fix one perspective: `V(s)` is expected **White** reward, in [-1,1]. White maximizes `V(next)`; Black minimizes it. Terminal reward is +1 for White's victory and -1 for Black's; intermediate reward is 0; gamma=1. Mars does not change the primary reward in the first experiment series.

```text
target = reward                         if terminal
         gamma * V(next)                otherwise

delta = target - V(current)

TD(0):
  theta += alpha * delta * grad V(current)

TD(lambda), accumulating traces:
  e = gamma * lambda * e + grad V(current)
  theta += alpha * delta * e
```

Do not differentiate through the bootstrap target. Reset eligibility traces before every game. With this fixed perspective, **do not invert the value sign every turn**. A later change to the current-player perspective is a new contract version requiring a separate formula derivation and tests.

Start with one learner and sequential self-play games, using the same current network for both sides. Initial settings: alpha=0.001, epsilon=0.05, lambda=0 first, then a separate lambda=0.7 experiment. These need validation and do not guarantee convergence. In inference, epsilon=0. Do not add a replay buffer, PPO, DQN, target networks, or reward shaping to the initial TD implementation.

Required tests: White/Black signs, terminal targets, no bootstrap after termination, trace reset, finite-difference gradient checks, no NaN, and identical decisions before/after loading. Compare updates on a small artificial trajectory with independently calculated results.

### 9.4 GA-MLP

The genome is the same MLP's parameters, with no gradient updates. Implement elitism, selection, and mutation. MLP crossover is a separate optional experiment; do not assume arbitrary mixing of independently trained neurons is beneficial. Describe the no-crossover mode accurately as mutation-based neuroevolution within the evolutionary experiment group.

### 9.5 Hybrid: TD + population-based training

Implement a PBT-like hybrid, not architecture search. Population-based training combines parameter training with selection and hyperparameter changes [R5]. Use synchronous rounds for reproducibility; this is an adaptation, not an exact replication of the paper's asynchronous algorithm.

Start with 8 agents of the same architecture. Each trains with TD in its own learner. After a fixed per-participant budget, evaluate frozen snapshots on the development set. Replace the bottom quarter with copies of the best participants: copy trained weights and vary alpha/epsilon/lambda within specified bounds. The initial version has no crossover and no required neural-weight mutation.

Record each child's ancestry, parameters, and replacement reasons. Replace participants only between games and reset traces. Evaluation and selection never modify parents. Training all participants, including discarded ones, counts toward the total budget. Architecture search and additional hybrids are outside the first research version.

## 10. Self-play, compute budget, and reproducibility

Use a bounded worker pool, not an unbounded goroutine per game. GA/arena parallelize independent games; Hybrid may parallelize independent participants. Multiple goroutines must not update one MLP without synchronization.

Initially guarantee reproducibility for the same code version, configuration, seed, and worker mode on the same platform. Do not promise bitwise identity across every CPU/compiler. For GA/arena, key seeds by game ID and aggregate in stable order so scheduling does not change results.

Dice, exploration, and evolution use independent random streams. Stored seeds, counters, and source state must permit resume. Tournament rolls are keyed by game, side, and turn number rather than the number of an agent's random calls.

Log separately: unique training games, environment decisions/transitions, selection games, final evaluation, forward evaluations, updates, wall-clock time, CPU time if actually measured, workers, platform, and versions. `wall_time × workers` is not measured CPU time. One game between two population members counts as one physical simulation.

Provide a configurable game-length guard. A reached limit is `truncated`, not a draw or loss. Report it separately and do not invent a terminal reward. Legality failures are experiment errors, not sporting results. For generation fitness evaluation, any truncated game stops the evaluation: do not select a generation from incomplete data. This policy was explicitly chosen by the user.

Job states: queued/running/stopping/completed/failed/interrupted. Stopping publishes a checkpoint at the nearest safe boundary. After a crash, preserve completed steps and mark unfinished runs interrupted. Test resume against an uninterrupted run.

## 11. Selecting strong versions and comparing methods

Separate development opponents/seeds for selection from final-test evaluation. Reusing final-test to choose a winner makes it development data; disclose this. Users may save any candidate, but a champion is defined only by the documented local protocol, not a global ranking.

Evaluate frozen models without exploration or training. Use independent pairs of games with sides swapped and predetermined dice streams. Shared streams reduce some differences in conditions; they do not remove randomness or make different games identical.

Show win rate, game count, mean points per game, mars wins, and statistical uncertainty. For paired games, bootstrap independent pairs for intervals; do not treat two correlated games as independent Bernoulli observations. Save the resampling configuration. Elo is an optional local presentation, not primary fitness or a global user rating.

Before automatic champion publication, fix the number of evaluation games and the criterion for outperforming the incumbent. Insufficient evidence leaves a candidate status. Frequent checks on one set do not prove significance; final confirmation is separate.

Compare GA-MLP/RL/Hybrid using the same rules, encoder, MLP size, objective, inference, and available compute. Plot budgets along two axes: environment interactions and measured time. Charge fitness/selection costs to GA/Hybrid rather than treating them as free. Meaningful conclusions require multiple independent training runs, initially 5 seeds, with variability shown rather than only the best seed.

A smoke run demonstrates pipeline functionality, not algorithm superiority. Avoid brittle CI requirements such as beating the heuristic after 100 games. Model strength is an experimental result and cannot be declared professional in advance.

## 12. Replays and analysis

A replay stores format/rules versions, initial position, opening contest, actual dice, full actions, bot IDs, outcome, and state hashes. A seed alone is insufficient. Playback does not require the original trainer and never asks a bot to select historical moves again.

The UI supports forward/backward steps, autoplay, and speed. Refresh restores the current session without new rolls. After completion, any position can be opened to request alternatives from different models. This is new analysis, not replay modification.

For linear evaluators, display feature contributions. For neural evaluators, display values and rankings without invented explanations of thoughts. Compare style metrics such as block length or home counts on the same positions or account for game phase; one average across different games does not demonstrate discovery of a strategy.

## 13. Milestones and acceptance criteria

### M0 — inspection and specification

Inspect the repository and tools; record the stack and dependency versions; create RULES.md, ARCHITECTURE.md, PROGRESS.md, and configurations. Save lockfiles. AGENTS.md should contain stable rules, commands, and links, not the entire plan [R6].

**Done:** an executable minimal Go project, frontend scaffold, check commands, and a consistently documented profile. No undisclosed standard-backgammon behavior.

### M1 — engine and baseline simulations

Implement coordinates, state, full-turn generation, application, results, PRNG injection, Random/Heuristic, and replay recording. Unit/property tests precede profiling.

Position fixtures must cover both sides: global-zero crossing; a point with a single opposing checker; both dice orders; legal/illegal intermediate landings; ordinary head limits; opening exceptions and obstructions; no exception later; incomplete doubles; maximum dice use; larger-die choice; pass; six-point blocks during intermediate steps; the last checker entering home and bearing off in the same turn; exact and overshooting bear-off; prohibited early bear-off; immediate termination; mars.

Check conservation of 15 checkers including off, no overlapping colors, increasing progress, and input immutability. Verify generator completeness with an independent slow enumerator on bounded fixtures. Checking only returned actions proves soundness, not completeness. Test symmetry by swapping colors and rotating the board.

**Done:** tests and race detector pass; fixed simulation series produce no illegal actions; replays exactly restore final states; truncation is reported separately.

### M2 — human play and library

Implement the API, SVG board, server continuations, confirmation, idempotency, built-in cards, model save/load, history, and session recovery. Support narrow screens and keyboard access to main buttons.

**Done:** a user finishes a game against Heuristic, refreshes without losing dice, restarts the server, and sees the library. Repeated requests never execute twice. The UI shows only real data.

### M3 — first complete trainable product

Add GA-linear, job manager, bounded resources, checkpoint/resume, training screen, manual candidate saves, and independent evaluation.

**Done:** a short smoke run trains actual coefficients; the bot saves, reloads after restart, and plays without a trainer. Continuous and interrupted/resumed runs match in a deterministic test. Checkpoints preserve published models. Users can play against a saved bot while another run trains.

### M4 — neural evaluator and RL

Add the encoder, MLP, gradient checks, TD(0), then TD(lambda). Connect them to the same library and UI. Test the learner on artificial trajectories and real smoke games.

**Done:** weights actually update; signs, terminal behavior, and traces are correct; serialization preserves behavior; short runs are not presented as strong agents.

### M5 — GA-MLP and Hybrid

Add evolution of the shared network's parameters and a synchronous PBT-like cycle. Implement lineage and total counters.

**Done:** three methods use one network; participant replacement copies trained weights without changing parents; all budget is accounted for; snapshots support ordinary play.

### M6 — research mode

Add development/final separation, paired-match schedules, uncertainty intervals, a multi-seed runner, real charts, and a champion publication protocol.

**Done:** one configuration runs a bounded end-to-end experiment; artifacts include exact models, seeds, versions, and results. Demo data does not preassign Hybrid as the winner.

### M7 — analysis and release

Add shared-position comparisons, export/import, stronger UI tests, packaging, CI, and documentation. Do not publish artifacts on the user's behalf without a separate request.

**Done:** the release binary serves the UI without Node.js; bot archives transfer to a clean data directory; corrupt/unsafe archives are rejected; a new user follows train → save → play using README.

## 14. Commands and final checks

Design a CLI along these lines. Record exact syntax during implementation; these are proposed commands, not an existing tool's commands:

```bash
evonardy serve --addr 127.0.0.1:8080 --data-dir ./data
evonardy train --config configs/ga-linear-smoke.json --data-dir ./data
evonardy train --config configs/rl-smoke.json --data-dir ./data
evonardy train --config configs/hybrid-smoke.json --data-dir ./data
evonardy resume --run <run-id> --data-dir ./data
evonardy bots list --data-dir ./data
evonardy evaluate --config configs/evaluation-smoke.json --data-dir ./data
```

Do not run offline train while a server owns the same directory; create training through the UI/API instead. CLI and UI must explain this conflict.

Provide `make dev`, `make build`, `make test`, `make smoke`, and `make bench`. Backend checks: gofmt, `go vet ./...`, `go test ./...`, `go test -race ./...`. Frontend: typecheck, unit tests, and build. A separate e2e test exercises the library, a game, and saving after short training. Lock the browser-test runner and its dependencies.

CI does not train a million games. Long experiments run separately. Benchmarks measure generation, model evaluation, and simulations on documented hardware. Never invent numbers or promise them for every computer.

After each milestone, report implemented behavior, changed modules, checks actually run, limitations, and the next step. State unavailable checks explicitly. A scaffold or attractive UI alone does not complete the full plan.

## 15. Implementation sources

These are sources for rules and methods. Architecture, milestones, API, encoder, and numerical presets above are project decisions.

- **[R1]** FNR, Rules of the Sport of Nardy, September 14, 2026 edition: `https://sportnardy.ru/images/NARDI/Pravila_Nardy_2026.pdf`. Articles 19, 20, 27, and 28 are especially relevant to the engine; application differences from tournament regulations are listed above.
- **[R2]** Gerald Tesauro, *Temporal Difference Learning and TD-Gammon*, Communications of the ACM, 1995, the author's article in an authorized republication: `https://www.bkgm.com/articles/tesauro/tdl.html`.
- **[R3]** Go, embed package: `https://pkg.go.dev/embed`.
- **[R4]** Vite, Getting Started: `https://vite.dev/guide/`.
- **[R5]** Jaderberg et al., *Population Based Training of Neural Networks*, 2017: `https://arxiv.org/abs/1711.09846`.
- **[R6]** OpenAI, AGENTS.md project instructions: `https://developers.openai.com/codex/guides/agents-md` (at preparation time redirects to `https://learn.chatgpt.com/docs/agent-configuration/agents-md`).
