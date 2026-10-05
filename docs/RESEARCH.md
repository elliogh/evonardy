# Research contracts

## Statistics

`seed-stratified-pair-percentile-v1` accepts completed, ordered pairs: the target
plays White then Black against the same frozen opponent with the same dice seed.
A pair is the sampling unit; its two correlated games are never independently
resampled. Missing sides, duplicate opponent/seed pairs, changed schedules and
truncations have no estimate.

The estimator gives each training seed and opponent equal weight. A bootstrap
replicate samples training-seed clusters with replacement, then samples dice pairs
within each selected seed/opponent stratum. Method differences use the same
resampling indices and require matching schedules. Reports retain the bootstrap
seed, resample count and confidence level. Percentile bounds use linear
interpolation at `(n - 1) * quantile`. This is a transparent descriptive estimate,
not a guarantee of nominal coverage with a few seeds or degenerate samples.

The default is five independent training seeds, 2000 resamples and a two-sided
95% interval. Record every run, not only the best seed. Report win rate, signed
mean points per physical game and Mars win rate. A method difference is A minus B
on the shared opponent schedule; it is not a head-to-head probability.

The paired resampling and explicit percentile method follow the
[SciPy bootstrap reference](https://docs.scipy.org/doc/scipy/reference/generated/scipy.stats.bootstrap.html).
The need to show uncertainty across independent training runs is discussed in
[Deep RL at the Edge of the Statistical Precipice](https://proceedings.neurips.cc/paper/2021/hash/f514cec81cb148559cf475e7426eed5e-Abstract.html).
No SciPy dependency is required by the application.

## Champion confirmation

`single-heldout-incumbent-v1` declares the incumbent, held-out schedule, pair quota,
minimum independent pairs, bootstrap and required margin before the first final
game. Select one frozen candidate using development data only. Run one final batch
against one frozen incumbent. Confirmation requires the lower confidence bound
above `0.5 + minimum_margin`, enough independent pairs and a nondegenerate
bootstrap. Otherwise the model retains candidate status. The default minimum is
100 pairs; the configurable minimum cannot be below 30. Bounded smoke deliberately
uses fewer pairs and cannot confirm a champion.

Confirmation records a local result, without modifying model files or replacing
a global champion. Repeatedly choosing candidates using the final results turns
those results into development data; they cannot supply independent confirmation.

## Experiment execution

`research-experiment-v1` runs methods in declared order and every training seed in
order, without a preferred winner. GA-MLP and Hybrid use the existing population
trainer; TD uses the existing sequential self-play trainer. All share the versioned
56→32→1 network, White win objective and frozen zero-exploration inference. GA's
initialization scale must be one, matching TD and Hybrid. Algorithm-specific
hyperparameters remain explicit: this does not make equal game counts equal
compute budgets.

Each complete seed run publishes an immutable inference model, records its exact
manifest, then evaluates it against the opponents frozen at experiment creation.
Development schedules are shared across methods and distinct across training-seed
indices. The highest development win rate selects one model, with declared
method/seed order breaking ties. Method estimates and paired differences still
include every seed. The selection lock is saved before any final game.

Dice pairing uses `research-phase-paired-v1`, separate development/final domains,
shared per-side dice schedules and swapped target colors. Training seeds and both
evaluation roots must differ. Shared dice reduce variance; they do not guarantee
identical positions or game lengths. Final schedules already reserved in the same
data directory are explicitly labeled `reused-development`; they cannot confirm
a champion, even when the numeric criterion passes. A fresh directory does not
make previously inspected final data independent; keep experiment provenance.

One research executor advances at completed learner-game or selection/evaluation
wave boundaries. Stop, process interruption and resume preserve committed random
schedules and work. Resume requires the recorded OS/architecture and Go version.
Checksummed checkpoints validate configs, frozen policies, raw schedules and
computed evidence; changed model packages or reports fail loading. User commands
have optimistic versions and idempotency receipts. Capacity is 100 experiments,
20 seeds, four unique methods, 200000 total physical games and 200000000 turn slots;
evaluation and resampling have tighter limits. These are admission bounds, not
recommendations for normal verification.

Counters charge every physical training and selection game once, including
discarded population members. Each seed reports training/selection counts,
completed/truncated games, decisions, forward evaluations, mutations and updates.
Development and final evaluation have separate counters and measured durations.
`wall_seconds` is the sum of measured trainer/evaluator calls; it excludes queue
waiting, persistence and publication overhead. CPU time is not measured or
reported. Worker counts are in the exact configs. The server can also run ordinary
jobs; use the CLI with an exclusive data directory for isolated timing comparisons.
Truncated selection/development/final batches fail with actual committed work and
no fake draw, fitness, estimate or confirmation. TD learner truncations bootstrap
as defined by the training contract.

Completed reports are immutable JSON under
`experiments/<id>/reports/<sha256>.json`; the checkpoint retains their digest.
Reports include exact expanded per-seed configs, model manifests, raw outcomes,
seeds, version contracts, platform and build revision (explicitly `unknown` when
not available). Full model parameters stay in the immutable local library. A
confirmed result is scoped to this protocol and incumbent, not proof that a
method is universally superior.

```bash
go run ./cmd/evonardy experiment --config configs/research-smoke.json \
  --name "Bounded research" --data-dir .cache/research
# SIGINT saves the current safe boundary; use the ID printed in the JSON.
go run ./cmd/evonardy experiment-resume --experiment EXPERIMENT_ID \
  --data-dir .cache/research
```

`GET /api/experiments/config` returns the full five-seed default. Supplying a config
replaces array members entirely: every method needs its complete method-specific
config. `experiment-resume` always uses the saved config. Research JSON uses
numeric uint64 seeds; clients that cannot represent values above 2^53 exactly
should keep seeds within their numeric precision.
