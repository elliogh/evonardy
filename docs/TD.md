# Neural temporal-difference learning

Algorithm: `td-zero-v1`; random contract: `keyed-pcg-td-v1`. The numerical
network/encoder contracts are in [NEURAL.md](NEURAL.md) and [ENCODER.md](ENCODER.md).
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
forward/update counts are checked against observed actions. TD(lambda), durable
CLI/API jobs, and the neural training UI remain subsequent M4 tasks.
