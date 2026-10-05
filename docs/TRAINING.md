# Training and independent evaluation

GA-linear changes the nine coefficients of `long-nardy-features-v1`. Neural
GA-MLP, TD and Hybrid share the encoder, `56 → 32 → 1` float64 tanh network,
fixed White win objective and frozen greedy inference policy. Go supplies legal actions. The ruleset,
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
workers 1–8, and turn guards 1–10000. GA combined budgets are capped at 200000 games
and 200000000 turn slots. Evaluation allows at most eight opponents, 1000 games,
and 5000000 turn slots. A directory holds at most 200 jobs; checkpoints and
generation files are bounded to 16 MiB and control receipts to 4096 per job.
Archive cleanup is manual while the data directory is closed.

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
and league comparisons belong to M6.
Charts contain saved generation metrics; candidates are promoted only by the user.

## Neural TD training

TD(0) and accumulating TD(lambda) learn the shared `56 → 32 → 1` tanh network
through sequential real self-play. Both colors use a fixed White value perspective;
each turn applies one plain-SGD TD update. Terminal reward is +1/-1 without mars
multipliers. Full numerical, exploration, trace, and checkpoint contracts are in
[TD.md](TD.md). Alpha/epsilon/lambda are learning settings, not strength estimates.

The CLI shares the existing queue, controls, immutable publication, and independent
evaluation. Stop the server before opening its data directory offline:

```bash
bin/evonardy train --algorithm td0 --config configs/td-zero-smoke.json \
  --data-dir ./data --save-name "My TD(0) bot"
bin/evonardy train --algorithm td-lambda --config configs/td-lambda-smoke.json \
  --data-dir ./data --save-name "My TD(lambda) bot"
bin/evonardy resume --run <run-id> --data-dir ./data
bin/evonardy evaluate --bot <saved-bot-id> --config configs/evaluation-smoke.json \
  --data-dir ./data
```

Both presets run four guarded games. CLI defaults are seed=42, games=32,
max_turns=1200, alpha=0.001, epsilon=0.05; `td-lambda` additionally defaults to
lambda=0.7. `td0` requires lambda=0; `td-lambda` requires positive lambda. A durable
job permits 1–10000 games, 1–10000 turns per game, and at most 200000000 turn slots.
There is one sequential learner, with no workers/population/generation settings.

`--save-name` publishes the final game checkpoint after full completion; earlier
checkpoints can be saved through the API. Resume uses the saved algorithm/config
without another `--algorithm` flag. SIGINT/SIGTERM commits the current game boundary
before returning a resumable snapshot. Startup leaves interrupted jobs available
for explicit resume. Truncated self-play is counted separately, has no winner or
terminal reward, and retains its last ordinary bootstrap update. It does not
produce an invented fitness score or fail TD training.

Counters report physical games, completed/truncated games, decisions, actual neural
forward evaluations, and `updates` (one per full turn). GA mutation/crossover
counters remain zero. Each game's history reports mean absolute TD error and
actual outcome/work; it is not calibrated win probability or an independent
evaluation. Frozen neural packages can be evaluated or played from My bots without
a learner.

In **Training**, select **TD(0)** or **TD(lambda)** under **Training method**.
Set Games, Seed, Learning rate (alpha), Exploration (epsilon), and, for traces,
Trace decay (lambda). Advanced settings contains the per-game turn guard. The
neural smoke preset uses four games; defaults and durable limits match the CLI.
Population/workers/variation controls are absent from the TD form.
**Stop and checkpoint** finishes the current bounded game; **Resume run** uses
the exact saved configuration after restart.

Run details show committed game checkpoints, actual decisions/forwards/updates,
completed versus truncated games, and active wall time. **TD learning error**
plots each game's measured mean absolute TD error; rendering retains only the
latest 200 points while the snapshot retains full bounded history. This diagnostic
does not measure playing strength or calibrated win probability. **Save neural
snapshot** publishes the latest observed game checkpoint while learning can
continue. The saved copy appears in My bots, with independent Evaluate and Play
controls. Reopen the data directory to load the same immutable package.

## Training games on the board

The Training screen includes automatic playback of actual matches sampled from
committed TD games or GA worker batches. It shows the participants, colors, opening dice,
full-turn moves, board positions, and terminal result. Pause playback, move to a
previous/next turn, drag the turn slider, or choose Slow/Normal/Fast speed.
Follow training moves to the newest available sample after the current replay
ends; Show latest game switches immediately.

Visual playback is independent of training. Training may finish many games while
one replay plays, so this is explicitly sampled playback, not a claim to show every
match in real time. No extra games or inference are performed for observation.
The latest actual replay is included in each checkpoint and restored after restart.
Neural runs identify the preview as TD self-play: the live learner plays both
colors. The game number is its committed index, rather than a GA generation.
Older checkpoints stay compatible; previously completed runs that did not record
a preview have none. Resumable older runs capture new samples after resuming.
Positions and legality remain authoritative in Go. A paused or hidden viewer never
pauses the job, and watchers keep only a bounded current/latest pair of replays.

## Neural populations: GA-MLP and Hybrid

Select **GA-MLP** or **Hybrid** in Training. Both use the same encoder, network,
reward and winning-action priority as TD. Their selection policy is frozen,
greedy (epsilon=0) and maximizes White value / minimizes Black value. Selection
uses the same fixed paired Heuristic/Random development seeds as GA-linear;
independent evaluation uses a separate seed domain and never updates learners.

```bash
bin/evonardy train --algorithm ga-mlp --config configs/ga-mlp-smoke.json \
  --data-dir ./data --save-name "My evolved neural bot"
bin/evonardy train --algorithm hybrid --config configs/hybrid-smoke.json \
  --data-dir ./data --save-name "My Hybrid bot"
```

GA-MLP (`ga-mlp-v1`) evolves all 1,857 parameters without gradients. Xavier-normal
initial networks are scaled by `initial_sigma`; defaults are eight candidates,
four generations, two pairs per opponent, elite fraction 0.1, tournament size 3,
initial scale 1, mutation scale 0.02, two selection workers and 1200 turns. Stable
ranking uses win/loss fitness, with mars recorded only as an auxiliary statistic.
Elites copy weights. Every other child selects one tournament parent and adds
independent Gaussian parameter mutations clamped to inference bounds ±1e6.
There is no crossover. Counters record changed parameters, excluding initialization.
The smoke preset evaluates two generations of four candidates: 32 selection games.

Hybrid (`hybrid-sync-v1`) is synchronous PBT-like adaptation. Exactly eight
participants own sequential TD(lambda) learners. Each round first trains every
participant for `games_per_round` games, then evaluates frozen weight copies on
the paired development schedule. The current implementation dispatches training
games sequentially; `workers` bounds parallel selection only. Truncated training
games bootstrap normally and have no terminal reward. Any truncated selection
game fails the round without fitness or replacement.

Between nonfinal rounds, the six highest ranked participants survive and the
bottom two copy the **trained weights** of ranks one and two respectively.
Survivors and replacements get fresh lineage IDs; immutable round archives record
parents, replaced IDs, reasons and before/after hyperparameters. There is no
crossover or neural-weight mutation. Alpha is multiplied by 0.8 or 1.2; epsilon
and lambda change by ±0.02 and ±0.05 respectively, then clamp to explicit bounds.
Defaults are alpha=0.001 within [0.00001,0.1], epsilon=0.05 within [0,0.3],
lambda=0.7 within [0,0.95], four rounds, eight training games per participant per
round, one pair per opponent and two selection workers. Bounds are configurable
within [0,1], with strictly positive alpha minimum and min < max. Eligibility
traces reset at every game boundary, including replacement and resume.
The two-round smoke preset uses one training game per participant per round:
16 training + 64 selection = 80 actual games, including discarded learners.

Population details separate cumulative training and selection games, show current
participant progress, and plot only complete selection rounds. Choose an evaluated
generation and any candidate to save its frozen neural package. Historical Hybrid
rounds also show replacement reasons and hyperparameter changes. Parameters stay
private to archives/models; public summaries contain identity, lineage and fitness.
The viewer samples real learner or selection games without running extra inference;
Hybrid keys distinguish training and selection indices within each round.

The random contract is `keyed-pcg-neuro-v1`: initialization uses the shared network
initializer; selection/mutation and Hybrid hyperparameter adaptation have independent
named PCG streams. A Hybrid training-game seed derives from run seed, round,
participant slot and game index. Its TD opening, dice and exploration streams remain
separate. Checkpoints retain this recipe, fixed opponents, all participant weights,
hyperparameters, phase/progress, complete round metrics and all actual work.
New population jobs record platform, Go runtime, configured workers, ruleset, encoder,
network and random contract under `execution`. Same-platform worker counts and
stop/reopen/resume reproduce results and parameters;
cross-platform bitwise equivalence is not promised. Active wall time is measured;
CPU time is not inferred from it.

Stop/shutdown finishes one learner game or bounded selection wave. Every such
boundary is checkpointed. Completed round archives are immutable and checksum
verified before publication; archive-ahead crash recovery requires byte-identical
reproduction. Published models never alias learners. Resume retains the entire
configuration and participant progress. GA-MLP has the existing GA limits; Hybrid
has 1–500 rounds, 1–1000 games per participant per round and the same overall
200000-game / 200000000-turn-slot caps, counting both training and selection.
Smoke/browser scenarios validate bounded execution, persistence and playable
snapshots, not relative strength. M6 adds systematic method comparisons.
