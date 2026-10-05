# Neural position encoder

Version: `long-nardy-encoder-v1`. The Go `internal/encoder` package exposes
`Vector [56]float64` and `Encode(game.Position) (Vector, error)`. This input is
shared by the planned RL, GA-MLP, and Hybrid evaluators. It is separate from
the nine-value `long-nardy-features-v1` linear baseline.

## Input and ordering

Encode the authoritative full-turn position **before the next roll**, including
positions after completed actions and terminal positions. Do not encode a draft
preview. `game.ValidatePosition` checks the ruleset and position invariants;
invalid input returns an error and a zero vector. The public position type cannot
distinguish a draft from a completed turn, so callers enforce the boundary.

All indices are zero-based. Point counts use global physical points `0..23`
for both colors, matching `Position.Checkers`; never rotate the board by the
side to move or reorder Black's points along its route. Each color has 15 checkers.

| Indices | Values |
| --- | --- |
| 0–23 | White checkers at global points 0–23, divided by 15 |
| 24–47 | Black checkers at global points 0–23, divided by 15 |
| 48, 49 | White, Black borne-off counts, divided by 15 |
| 50, 51 | Side to move: White, Black one-hot |
| 52, 53 | White, Black first-turn-pending flags: 1 when `FirstDone` is false |
| 54, 55 | Starting player: White, Black one-hot |

Every valid position produces exactly 56 finite values in `[0,1]`. Encoding is
deterministic, has no I/O or randomness, and does not mutate the position. Current
dice, future dice, private random streams, training state, and model weights are
not inputs. The same board with a different side to move is a different vector.

For White's initial position, indices `0, 36, 50, 52, 53, 54` are 1 and all others
are 0. For Black's initial position, indices `0, 36, 51, 52, 53, 55` are 1;
Black's head stays at global point 12 (input index 36).

## Value perspective and compatibility

`encoder.ValuePerspective` is `white`: the future network estimates White's win
reward in `[-1,1]`, with White maximizing and Black minimizing the same value.
The encoder does not compute a value or change terminal reward semantics.

Changing input ordering, normalization, flags, or perspective requires a new
encoder version and explicit model compatibility checks. Adding this encoder
does not change the game ruleset, existing linear models, or their manifests.
Neural inference, model publication, and learning are subsequent M4 tasks.
