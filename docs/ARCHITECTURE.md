# Architecture

The current implemented iteration is M0–M1. M2+ modules are added together with
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
- `storage`: data-directory locking and atomic replay publication; one writer.
- `cmd/evonardy`: simulate/replay CLI. Training and human-play commands are unavailable.
- `web`: React scaffold that explicitly describes unavailable human play and training.

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

M2 adds app/httpapi/library and server sessions. Dice and drafts belong to the
session; rules stay in game. Mutating commands use command_id + expected_version.
M3 adds a shared CLI/server training lifecycle, immutable inference packages,
and GA-linear. Any truncated fitness game stops the generation's evaluation;
selection from an incomplete evaluation set is prohibited by the user's decision.
MLP/RL/Hybrid follow a completed M3.
