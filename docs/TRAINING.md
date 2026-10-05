# GA-linear training and independent evaluation — M3

Training changes the nine coefficients of `long-nardy-features-v1`. Inference
uses the existing frozen linear evaluator and Go legal actions. The ruleset,
winning-action priority, and stable tie behavior are unchanged.

## Start, stop, and keep a candidate

Open **Training**, choose a seed and budget, and start a GA-linear run. The smoke
preset uses population 4, two generations, one pair per opponent, and two workers:
32 games. Stop retains the completed game prefix, including unfinished generation
work. **Resume run** continues the exact saved configuration.

After a whole generation completes, choose any candidate from that or an older
generation, give it a name, and **Save candidate**. Training can continue. The
library receives an immutable inference package. Identical strategies share a
content identity and keep the existing card's name. Play and evaluation use
frozen package data without a trainer dependency.

The offline CLI uses the same manager and requires exclusive directory access.
Stop the server before using its data directory with these commands:

```bash
bin/evonardy train --config configs/ga-linear-smoke.json --name "First experiment" \
  --data-dir ./data --save-name "My GA bot"
bin/evonardy resume --run <run-id> --data-dir ./data
bin/evonardy evaluate --bot <saved-bot-id> --config configs/evaluation-smoke.json \
  --data-dir ./data
```

`train` requires a config file; omitted fields use GA defaults. `--save-name` saves
the best candidate only after full completion. The UI provides manual selection
during a run. SIGINT/SIGTERM finishes the current bounded batch, checkpoints, and
prints a resumable snapshot. Failed jobs return a nonzero exit code. `resume`
works for stopped/interrupted training and evaluation jobs.

## Fitness and variation

Defaults: population 64, 20 generations, two pairs per opponent, two workers,
1200 full turns per game, elite fraction 0.10, tournament size 3, initial Gaussian
spread 0.40, and mutation scale 0.15. Budgets and variation are configurable.

Candidate zero starts at Heuristic's actual weights. Other initial candidates add
independent Gaussian perturbations. Coefficients are clamped to [-10,10]. Each
candidate plays both colors against fixed Heuristic and Random policies. All
candidates and generations receive the same development pair schedule.

Fitness is `(wins - losses) / completed_games`, with terminal rewards +1/-1.
Signed points and mars wins are auxiliary metrics. Ranking occurs only after
the entire generation completes; ties retain population order. Elitism copies
`ceil(population * elite_fraction)` genomes. Tournament selection chooses parents
from the ranking. Uniform crossover chooses a parent independently per coefficient;
Gaussian mutation changes each nonelite child's coefficient. Lineage records
parent candidate IDs.

Any truncated selection game fails the job with actual counters and results.
It produces no reward, partial fitness, selection, or new generation. Earlier
complete generations remain available to save. Legality failures are job errors.
Failed jobs cannot resume with a different budget.

## Reproducibility and persistence

Contracts are `ga-linear-v1`, `keyed-pcg-ga-v1`, the ruleset, and feature version.
State contains config, fixed opponent policies, population, lineage, completed
prefix, metrics, counters, and generation index. Independent PCG streams derive
from seed, label, and index: initialization, selection, crossover, mutation,
development environment, and independent evaluation have separate domains.
Inside each match, dice and agent choices use separate streams. Opening and
per-side dice are shared across both color assignments in a pair.

No mutable PRNG spans batches. The serialized seed contract and schedule indices
are the full generator state required for resume. Ordered results and generation
boundaries make worker response order irrelevant. With the same implementation
and platform, workers=1/2 and continuous/resumed runs produce identical genomes,
game scores, metrics, and work counters. Wall time and timestamps naturally differ.
Cross-platform bitwise portability is not promised.

One job runs at a time, with at most eight match workers and one batch in flight.
Every completed batch is durably checkpointed. Stop finishes that batch and stops
dispatching. Shutdown does the same and marks the run interrupted. Startup marks
formerly queued/running/stopping jobs interrupted; the user chooses what to resume.
A crash can lose only the uncommitted batch, replayed from the saved prefix.
Durable counters do not repeat completed steps.

Atomic checkpoints use SHA-256 envelopes. Generation archives are immutable and
referenced by checksum, checked before candidate publication. A crash after an
archive write but before checkpoint advancement is repaired by reproducing that
exact archive. Loading rejects incompatible versions, shapes, opponents, schedules,
and counters. Checkpointing and training never overwrite published models.

Training allows population 4–128, generations 1–500, pairs per opponent 1–32,
workers 1–8, and turn guards 1–10000. Combined budgets are capped at 200000 games
and 200000000 turn slots. Evaluation allows at most eight opponents, 1000 games,
and 5000000 turn slots. A directory holds at most 200 jobs; checkpoints and
generation files are bounded to 16 MiB and control receipts to 4096 per job.
Archive cleanup is manual in M3 while the data directory is closed.

Counters measure completed/truncated games, full-turn decisions, linear forward
evaluations (including opponents), changed coefficients from mutation, and nonelite
child crossovers. Initial perturbations are not counted as mutations. Active wall
time covers each batch through matches, fitness, variation, and generation
publication; queue time and checkpoint writes are excluded. CPU time is not
measured; wall time is never multiplied by workers.

## Independent evaluation

Open **Evaluate**, choose a model and opponents, and set the pair budget. The
manager freezes all policies before queuing. Pair seeds use
`evaluation/independent-v1`, separate from `ga/development`, even with the same
numeric seed. Evaluation never updates training fitness.

After every scheduled game completes, results report wins, game count, win rate,
mean signed points, and mars wins. Stopped batches have no aggregate result and
can resume. Truncated batches fail with actual records and no aggregate result.
My bots links to each model's latest completed independent result. Smoke batches
verify execution and persistence, not playing strength. Confidence intervals,
league comparisons, neural evaluators, RL, and Hybrid are later milestones.
Charts contain saved generation metrics; candidates are promoted only by the user.

## Training games on the board

The Training screen includes automatic playback of actual matches sampled from
completed worker batches. It shows the candidate, opponent, colors, opening dice,
full-turn moves, board positions, and terminal result. Pause playback, move to a
previous/next turn, drag the turn slider, or choose Slow/Normal/Fast speed.
Follow training moves to the newest available sample after the current replay
ends; Show latest game switches immediately.

Visual playback is independent of training. Training may finish many games while
one replay plays, so this is explicitly sampled playback, not a claim to show every
match in real time. No extra games or inference are performed for observation.
The latest actual replay is included in each checkpoint and restored after restart.
Older checkpoints stay compatible; previously completed runs that did not record
a preview have none. Resumable older runs capture new samples after resuming.
Positions and legality remain authoritative in Go. A paused or hidden viewer never
pauses the job, and watchers keep only a bounded current/latest pair of replays.
