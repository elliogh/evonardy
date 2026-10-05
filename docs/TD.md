# Neural temporal-difference learning

Algorithms: `td-zero-v1` and `td-lambda-v1`; random contract: `keyed-pcg-td-v1`.
The numerical network/encoder contracts are in [NEURAL.md](NEURAL.md) and [ENCODER.md](ENCODER.md).
`training.NewTDLearner` owns one sequential float64 learner;
`Update(current, next)` consumes authoritative full-turn positions. It validates
positions but does not generate or certify a move; the real self-play runner
obtains legal actions from Go and verifies each with `ApplyTurn`.

## TD(0) objective and update

Values always estimate White's reward. White maximizes and Black minimizes the
same next-state value. Gamma is fixed at 1; intermediate reward is 0. A White
terminal victory has reward +1 and a Black victory -1, including mars. There is
no value-sign inversion between turns, reward shaping, or replay buffer.

```text
value, gradient = network.ValueGradient(encode(current))
target = +1 or -1                         if next is terminal
         network.Forward(encode(next))   otherwise
delta = target - value
theta += alpha * delta * gradient
```

The target uses pre-update parameters and is treated as a constant. Terminal
targets never bootstrap; current terminal positions cannot be updated. Invalid
inputs or nonfinite/out-of-bounds updates return errors without changing weights.
`Parameters()` returns a copy for frozen publication; published models never
share learner memory. The learner is sequential and must not be shared between
workers without synchronization. Inference imports no learner or training state.

Initial settings are alpha=0.001 and epsilon=0.05. Alpha must be finite in
`(0,1]`; epsilon must be finite in `[0,1]`. These settings demonstrate a working
method, not convergence or strong play. Inference remains epsilon=0.

## Real self-play and work accounting

`NewTD(TDConfig)` initializes the shared network with its independent initialization
stream. `TrainTDGame(context, TDState)` plays one actual game sequentially with
the same current network for both colors, updating after each completed turn.
Epsilon exploration selects a uniform legal action, with immediate wins still
taking priority. The draw uses the upper 53 bits of a keyed PCG Uint64 divided
by `2^53`; action sampling uses only that exploration source.

Opening, dice, and exploration use distinct named streams under `td/game/<index>`.
Opening is keyed by attempt; dice/exploration are keyed by side and side-turn
number. Choice/gradient work cannot advance environment streams. Configuration,
seed, versions, parameters, next game index, history, and counters describe a
game-boundary state. Bitwise reproducibility is scoped to the same implementation
and platform.

Default budget is 32 games, each guarded at 1200 turns. Configuration bounds are
1–200000 games, 1–10000 turns per game, and at most 200000000 turn slots. Each
physical self-play game counts once; each turn counts one decision and one update.
Forward counts include successor ranking, the current value/gradient evaluation,
and the nonterminal bootstrap; exact terminal overrides cost no network forward.
History reports the actual mean absolute TD error, status, outcome, and work.

The turn guard produces `truncated`, with no winner or terminal reward; its last
observed nonterminal transition still uses ordinary bootstrap. Actual actions,
dice, positions/hashes, and outcome are retained in a verifiable sampled replay.
Cancellation/failure discards the unfinished game's updates and returns the
previous boundary state. Completed/truncated games commit parameters and counters
together. No partial game is counted as completed work.

Tests check all parameter updates on artificial White/Black/nonterminal transitions
against independent closed-form calculations, verify stop-gradient targets, and
exercise bounded real completed/truncated games. Actual replay reconstruction
checks legality and terminal behavior. Exploration changes cannot change dice;
forward/update counts are checked against observed actions. Durable CLI/API jobs
and the training UI use this boundary state.

## Accumulating TD(lambda) traces

`NewTDLambda(parameters, alpha, lambda)` extends the same learner with accumulating
eligibility traces. Gamma remains 1 and the target contract is unchanged:

```text
e = lambda * e + gradient
theta += alpha * delta * e
```

Lambda must be finite in `[0,1]`. `lambda=0` agrees exactly with TD(0);
`DefaultTDLambdaConfig()` selects a separate lambda=0.7 experiment. A positive
lambda selects `td-lambda-v1`, while omitted/zero lambda preserves `td-zero-v1`
and its original JSON/configuration behavior.

New learners and replacement networks start with zero traces. `ResetTraces`
clears traces without changing weights; `Traces()` returns a value copy.
Successful terminal updates apply the final trace, then clear it. Weight/trace
updates commit together; invalid input, overflow, or parameter-bound failures
change neither. Traces belong to one sequential learner and are never inference
or published-model parameters.

The self-play runner constructs a fresh learner at every game boundary, including
after truncation and restore. Boundary states therefore need no pending traces;
no unfinished game is checkpointed. Independent one-neuron trajectory calculations
verify every trace and parameter, lambda=0 behavior, terminal/manual/replacement
reset, atomic failure, and identical next games after boundary JSON restoration.

## Durable jobs and frozen saves

The shared job manager accepts `td_config` with optional matching `algorithm`.
CLI names are `td0` and `td-lambda`; wire/checkpoint names are `td-zero-v1` and
`td-lambda-v1`. The latter requires positive lambda in the CLI. See
[TRAINING.md](TRAINING.md) for commands and [API.md](API.md) for the public DTO.

One learner runs sequentially. After each game, the manager archives its actual
parameters/metric, then atomically publishes the boundary checkpoint. Stop and
shutdown finish the current bounded game before dispatching any more; an abrupt
crash discards uncommitted game work. Resume reconstructs all named random streams
from the saved seed, random contract, and next game index, with fresh zero traces.
There is no mutable PRNG or unfinished gradient state across a boundary.

The optimizer is sequential plain SGD, fixed by the algorithm version: gamma=1,
constant alpha, no momentum or adaptive moments. Checkpoints retain algorithm,
rules/encoder/network/random versions, config, parameters, completed game index,
history, actual work counters, latest real replay, immutable archive references,
and command receipts. Traces are empty and need no serialization here. A checksum
protects the complete private record; recovery validates state and replay contracts.

Durable jobs allow at most 10000 games to bound history in the shared 16 MiB
checkpoint. Pure runner bounds still apply. Each game archive contains 1857
parameters and one metric; total disk use grows with the chosen budget. Archives
are retained until manual cleanup with the data directory closed. Shared capacity
is 200 jobs and 4096 control receipts per job. No parallel learner workers exist.

Manual save chooses any committed game checkpoint, checks its checksum/contracts,
and publishes a separate immutable neural inference package. The live learner,
earlier packages, and random streams are untouched. A save can run while subsequent
games train. Same-platform continuous, safely stopped/reopened, and crash-recovered
runs are checked against identical final weights, histories, counters, and content
identities. Existing GA-linear records keep their original format and behavior.

The browser selects GA-linear, TD(0), or TD(lambda) with method-specific settings.
Neural progress counts game checkpoints and real TD updates; the diagnostic chart
plots mean absolute TD error for the latest 200 saved games. It is not a win
probability or strength estimate. A neural snapshot saves the latest observed
game boundary for independent evaluation or play from My bots. The training board
shows actual self-play replays with both colors identified as the current learner.
Watching and human sessions consume no learner random draws; tests compare an
observed/played/stopped/resumed run against uninterrupted self-play.
