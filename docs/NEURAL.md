# Shared neural value network

Version: `tanh-56-32-1-v1`. The Go `internal/neural` package implements a
`56 → 32 → 1` network with tanh hidden and output activations and float64
parameters. Its input is `encoder.Vector`; see [ENCODER.md](ENCODER.md) for
the versioned public full-turn position contract and White value perspective.
The numerical network accepts finite vectors without validating game semantics.

## Shapes and parameter ordering

The network has 1857 parameters. `New([]float64)` requires exactly that count,
rejects NaN and infinity, and copies the slice. The zero `Network` is valid and
returns zero for finite inputs. `Parameters()` returns a separate copy.

| Indices | Shape | Meaning |
| --- | --- | --- |
| 0–1791 | W1: 32 × 56 | Hidden weights, row-major: `W1[j,i] = theta[j*56+i]` |
| 1792–1823 | b1: 32 | Hidden biases |
| 1824–1855 | W2: 1 × 32 | Output weights |
| 1856 | b2: 1 | Output bias |

```text
h[j] = tanh(b1[j] + sum_i W1[j,i] * x[i])
V(x) = tanh(b2 + sum_j W2[j] * h[j])
```

`Forward` returns the value. `ValueGradient` returns the same value and
`dV/dtheta`, a `[1857]float64` gradient in the same ordering. It differentiates
the value, without a loss function or bootstrap target:

```text
q = 1 - V*V
dV/db2 = q
dV/dW2[j] = q * h[j]
r[j] = q * W2[j] * (1 - h[j]*h[j])
dV/db1[j] = r[j]
dV/dW1[j,i] = r[j] * x[i]
```

Evaluation does not mutate parameters or inputs and has no shared scratch state,
I/O, randomness, or dependency on training. Read-only evaluation may run
concurrently. A caller can update a parameter copy with
`theta += alpha * (target - value) * gradient`, then construct a new network;
the original stays unchanged. TD targets, traces, terminal rewards, and game
selection are separate tasks. The tanh range is `[-1,1]`; the network itself
does not override terminal values.

Inputs, preactivations, and returned gradients must be finite. Arithmetic
overflow is an error, even if tanh could saturate an infinite preactivation.
Errors return zero output and, for `ValueGradient`, a zero gradient. A nil
network is rejected. Input shape is fixed by the Go `encoder.Vector` type.

## Initialization and verification

`Initialize(seed, index)` creates its own keyed PCG stream with the label
`neural/initialization/v1`. It never consumes dice or exploration streams.
Weights use Xavier-normal scales: `sqrt(2/(56+32))` for W1 and
`sqrt(2/(32+1))` for W2. Draw W1 in row-major order, then W2; all biases
start at zero. The same seed/index reproduces the network for the same
implementation and platform; bitwise cross-platform equality is not promised.

Public tests compare sparse and dense forward fixtures with independently
calculated Python values, check a closed-form gradient, and check every
parameter using central differences at two inputs, including an actual encoded
position. The perturbation is `epsilon = 1e-6`; each derivative must satisfy
`abs(analytical - numerical) <= 1e-8 + 1e-5 * max(abs(analytical), abs(numerical))`.
This tolerance allows floating-point cancellation without accepting material
gradient errors. Tests also cover independent initialization, owned parameters,
an ordinary SGD step, concurrent evaluation, and invalid/nonfinite/overflow input.

Changing architecture, activation, parameter ordering, or initialization
semantics requires a new version and explicit compatibility checks. This module
supports frozen inference through the library and game agent described below.
Neural learning remains subsequent M4 work. Existing linear models remain unchanged.

## Frozen inference packages

`library.SaveNeural` copies a parameter snapshot into the existing immutable
bundle format. `manifest.json` identifies the evaluator with the network version,
the encoder through `features_version`, and the `[56,32,1]` architecture.
`model.json` contains `weights`, the 1857 parameters in the ordering above.
Inference uses the White perspective, win objective, zero exploration, and
`stable_first` ties. Identity covers the model checksum and these contracts.

Loading verifies strict JSON, checksums, versions, architecture, parameter count,
finite weights within ±1000000, and the existing file bounds: manifest/model
64 KiB each and metadata 1 MiB. Incompatible packages remain visible with their
reason. New neural support preserves existing linear/random identities and JSON.
Rename changes metadata only; learner updates and later saves cannot alter an
earlier bundle. Saving identical strategies deduplicates without overwriting.

`agent.Neural` owns its network. It consumes Go-generated legal successors,
maximizes their values for White and minimizes for Black, and selects the first
action on exact ties. Immediate victories override all network scores. Terminal
values are exact White `+1` or Black `-1`; nonterminal values use the network.
Known/future dice and exploration do not enter ranking. Measured inference counts
only actual network forward evaluations, excluding terminal overrides.

`library.Freeze` stores versioned parameters in a comparable, fixed-size value
array. JSON loading checks its exact shape rather than silently padding/truncating
arrays. Legacy policies omit new zero-valued fields, preserving old checkpoints.
Each game constructs its own inference instance without a learner or neural RNG.
