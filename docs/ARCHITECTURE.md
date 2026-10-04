# Architecture

The current implemented iteration is M0–M3. Later modules are added together with
behavior rather than as empty scaffolds. Go 1.25 with the standard library;
React 19 + TypeScript 5 + Vite 7, with Vitest 4. Exact frontend versions are
locked in `web/package-lock.json`. Go has no external dependencies, so a
go.sum file is not needed yet. All repository content and UI copy use English.

## Boundaries

- `game`: pure rules, value positions, full turns, and all UI paths/prefixes.
  LegalTurns merges identical final positions; LegalPaths retains paths.
  ApplyTurn validates individual steps and full-turn obligations.
- `features` / `agent`: normalized features and selection among supplied legal
  actions. Random has its own random source. Heuristic is deterministic and
  selects the first action on ties. Future dice are inaccessible.
- `random`: named deterministic PCG streams for the environment and agents;
  random state is serialized separately from the position.
- `arena`: bounded simulation workers, context cancellation, stable game IDs,
  separate completed/truncated outcomes, and scheduling-independent results.
- `replay`: version, actual opening/dice/actions, hashes, and outcome;
  playback verifies actions and hashes without inference or a trainer.
- `storage`: data-directory locking, immutable file/bundle publication, and atomic
  replacement of mutable sessions/metadata; one process owns the root.
- `library`: immutable inference packages, content identities, compatibility and
  checksum validation, built-in cards, and separate versioned display metadata.
- `app`: durable human sessions, private dice streams, bot turns, draft previews,
  command receipts, version conflicts, and replay archival. No training dependency.
- `httpapi`: bounded HTTP commands/snapshots and SSE notifications; explicit
  Host/Origin allowlists and strict JSON bodies. See [API.md](API.md).
- `training`: pure GA-linear generation state, paired schedules, full-generation
  fitness, elitism/tournament selection, crossover, mutation, and measured work.
- `jobs`: bounded durable queue, atomic checkpoints, immutable generation archives,
  interruption recovery, versioned controls, frozen publication, and evaluation.
- `cmd/evonardy`: serve, train, resume, evaluate, bots list/save, simulate, and replay.
  The server and offline commands use the same job lifecycle.
- `web`: a bot library, SVG board driven by legal continuations, turn history,
  resume through the URL, training, candidate saves, and independent evaluations.
  The Go server serves a separate Vite production build.

## Determinism and data

SHA-256 of the seed, stream label, and index produces a PCG seed. Dice are
keyed by game ID, side, and that side's turn number. Opening rolls use a separate
stream, and agent randomness has a different label. Results are aggregated by
game ID. Reproducibility is guaranteed for the same version, configuration,
and platform; bitwise portability across all architectures is not promised.

Replays use JSON. Each turn position is protected by SHA-256 of canonical JSON
with a fixed structure. Hashes verify integrity; they are not signatures or
proof that an external file is trusted. Version, ruleset, file size, and legality
are checked. Publication uses a temporary file, sync, and atomic creation
without overwriting. OS locks are released after process crashes. Published
replays are immutable.

## Later milestones

M4 adds the encoder, MLP, and RL. GA-MLP and Hybrid follow in M5; comparisons
and release work remain M6–M7. The completed GA-linear lifecycle and random
contracts are documented in [TRAINING.md](TRAINING.md).
