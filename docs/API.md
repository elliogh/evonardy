# Local API and persistence

The server binds to loopback only. HTTP carries commands and authoritative
snapshots; SSE carries small change notifications. Frontend types are in
`web/src/api.ts` and `web/src/jobs.ts`. Game behavior remains `long-nardy-fnr2026-nocube-v1`.

## Routes

| Method | Path | Request / result |
|---|---|---|
| GET | `/api/bots` | Bot cards, including unavailable incompatible packages |
| GET | `/api/bots/{id}` | Card and immutable manifest (null for built-ins) |
| POST | `/api/bots` | Save `{command_id, expected_version:0, source_bot_id, name}` |
| PATCH | `/api/bots/{id}/metadata` | Rename `{command_id, expected_version, name}` |
| GET | `/api/games` | Persisted game summaries, most recently updated first |
| POST | `/api/games` | Create `{command_id, expected_version:0, bot_id, human:0|1}` |
| GET | `/api/games/{id}` | Authoritative snapshot |
| POST | `/api/games/{id}/roll` | `{command_id, expected_version}` |
| POST | `/api/games/{id}/continuations` | Save draft `{command_id, expected_version, prefix:[{from,to,die}]}` |
| POST | `/api/games/{id}/turn` | Confirm `{command_id, expected_version, turn:{steps:[...]}}` |
| GET | `/api/games/{id}/events` | SSE `game.updated` with `{game_id, version}` |
| GET | `/api/replays/{id}` | Completed replay download; unfinished games return 409 |
| GET / POST | `/api/training/runs` | Run summaries / start with GA `config` or neural `td_config` (see below) |
| GET | `/api/training/runs/{id}` | Durable training snapshot |
| POST | `/api/training/runs/{id}/stop` or `/resume` | `{command_id, expected_version}`; checkpoint or continue |
| POST | `/api/training/runs/{id}/save-bot` | `{command_id, expected_version, generation, candidate_id, name}`; immutable bot card |
| GET | `/api/training/runs/{id}/generations/{n}` | Evaluated generation, ranked candidates, and actual scores |
| GET | `/api/training/runs/{id}/watch` | Latest sampled training replay and Go-reconstructed positions |
| GET | `/api/training/runs/{id}/events` | Throttled SSE revision notices |
| GET / POST | `/api/evaluations` | Summaries / start `{command_id, expected_version:0, bot_id, config}` |
| GET | `/api/evaluations/{id}` | Frozen-model evaluation snapshot and completed score prefix |
| POST | `/api/evaluations/{id}/stop` or `/resume` | Same versioned job control commands |
| GET | `/api/evaluations/{id}/events` | Throttled SSE revision notices |

IDs for the built-ins are `builtin/heuristic-v1` and `builtin/random-v1`.
Saved bot IDs are 64 hexadecimal characters. Commands use 1–128 ASCII letters,
digits, hyphens, or underscores; the UI uses UUIDs. A game's ID is its creation
command ID. Global points are 0–23 and `to:24` means bearing off. Players are
White=0 and Black=1. Requests use `application/json`, reject unknown fields and
trailing JSON, and have a 64 KiB body limit.

## Session commands

Phases are `moving`, `awaiting_roll`, and `finished`. Creation records the
opening contest and gives its winner the actual winning dice. If the bot starts,
it completes that opening turn before returning a human snapshot. Rolling is
allowed only in `awaiting_roll`. Dice, seeds, board assignments, arbitrary bot
actions, and changing opponents are not accepted from clients.

Every successful mutation increments the session version once. Confirming a
human turn includes the following bot turn in the same durable transaction.
The server validates the full turn and requires it to match the stored draft.
An empty turn is a pass only when the engine permits it. Drafts use engine
prefix validation and can be shortened or reset without changing assigned dice.

Receipts survive restart. Repeating the same command ID and body returns the
latest authoritative snapshot without applying it again, even if its expected
version is now old. Reusing that ID with a different operation/body or sending
a fresh command against an old version returns 409. Failed validation and
cancellation before the commit leave state unchanged. If the response is lost
after the commit, retry the exact same body and ID. Browser requests retry once
after network/server errors; conflicts refresh the current state.

Snapshots include `id`, `version`, opponent ID/name, human color, phase,
timestamps, `position`, `draft_position`, assigned `dice`, `remaining_dice`,
stored `draft`, `continuations:{next,complete}`, actual `opening` and `history`,
and an outcome only after completion. Private seeds and command ledgers are
never included. `position` is the last confirmed boundary; `draft_position` is
its engine-validated preview. The UI derives selectable sources/destinations
from `continuations.next`; it implements no legality rules.

## SSE, errors, and limits

Subscribe, then fetch the snapshot on notifications and reconnect. The initial
event contains the current version. SSE does not promise historical delivery;
one pending notification per subscriber is replaced with the latest update.
Keepalives are sent every 15 seconds. At most 64 streams and 32 ordinary HTTP
requests run concurrently. Ordinary request contexts expire after 10 seconds;
SSE writes have a 5-second deadline. Session capacity is 1000 games, 10000 full
turns per game, 20000 command receipts, and 16 MiB per persisted session.
Reaching a resource limit is an error, never an invented win or draw.

Errors have `{ "error": { "code": "conflict", "message": "..." } }`.
Invalid commands/JSON return 400, forbidden Host/Origin 403, missing resources
404, conflicts 409, excessive bodies 413, wrong media types 415, and busy or
cancelled requests 503. Internal failures return 500 with a retry instruction.
There is no broad CORS. The serve command allowlists its own Host/origin and
optionally a loopback `--dev-origin`; cross-site fetches are rejected.

## Files and compatibility

`games/<id>.json` is a private, versioned session snapshot written by atomic
replacement. It contains the seed, per-side turn counters, full actual history,
draft, and command receipts. Recovery verifies the ruleset, history hashes and
legality, current position, counters, phase, draft, and result. Dice and agent
streams have distinct labels; inference receives neither seeds nor future dice.
Completed sessions are canonical; their immutable `replays/game-<id>.json`
archive can be reconstructed after a failed archival write without rerolling.

`bots/<sha256>/` is published as a complete bundle with `manifest.json`,
`model.json`, and `metadata.json`. Strategy identity includes the model checksum,
ruleset, features, architecture, objective, value perspective, and inference
contract. Creation timestamp, display name, and ancestry do not change identity.
Saving identical strategies deduplicates and preserves existing metadata.
Manifest/model files stay immutable; rename atomically updates only metadata
and its command ledger. Save receipts live in `bots/commands/`, with a limit of
10000 fresh save commands; existing receipts remain retryable.

Format version 1 supports linear float64 weights (9 inputs → 1 value), the
uniform random baseline, and the `tanh-56-32-1-v1` neural evaluator. Neural
manifests use `features_version: "long-nardy-encoder-v1"`, architecture
`[56,32,1]`, and 1857 flat model weights in [NEURAL.md](NEURAL.md) order.
Neural inference ranks full-turn successors using White's value: White maximizes,
Black minimizes, immediate wins take priority, and terminal values are exact
`+1`/`-1`. It uses stable first-action ties and no exploration or random source.
Linear inference uses stable first-action ties,
White value perspective, win objective, and zero exploration. Random is itself
a stochastic strategy, with a separate per-turn agent source. The loader checks
format/rules/features, shapes, weight count, raw-file SHA-256, finite weights
within ±1000000, and timestamps. Manifest/model files are limited to 64 KiB each;
metadata is limited to 1 MiB and 2048 rename receipts. Incompatible models remain
visible with their reason and cannot play. Trained cards identify their source
run and generation; independent results remain separate artifacts. The data
directory remains exclusively locked by one process.

## Training and evaluation jobs

Job IDs equal creation command IDs. States are queued, running, stopping, stopped,
interrupted, completed, and failed. Config schemas, objectives, budgets, random
contracts, and checkpoint rules are in [TRAINING.md](TRAINING.md). Example JSON
configs are in `configs/ga-linear-smoke.json` and `configs/evaluation-smoke.json`.
Requests require a complete config; the CLI additionally merges config defaults.

GA requests retain `{command_id, expected_version:0, name, config}` unchanged;
optional `algorithm:"ga-linear-v1"` is accepted. Neural requests omit GA `config`
and supply `td_config:{seed,games,max_turns,alpha,epsilon,lambda?}` instead, with
optional `algorithm:"td-zero-v1"` (lambda=0) or `"td-lambda-v1"` (lambda>0).
Mixed configurations and mismatching/unknown algorithm names return 400. Neural
presets are `configs/td-zero-smoke.json` and `configs/td-lambda-smoke.json`;
resource/update/random contracts are in [TD.md](TD.md).

Neural snapshots have `config:null`, their versioned `algorithm`, `td_config`,
`td_history` (one actual metric per committed game), and, after the first game,
`neural_candidate:{id:"td-game-000001",game:1}` for the latest checkpoint.
`generation` is the committed game index, `generation_games` is the cumulative
committed game count, and `generation_budget` is the configured game budget;
there are no GA candidates/fitness/history. `counters.updates` records actual TD
updates and is omitted when zero for legacy compatibility. The shared save route
uses `generation:<game-index>` and matching `candidate_id:"td-game-NNNNNN"` to
publish any completed game checkpoint. The GA generation GET route is inapplicable
to neural runs and returns 400. Public responses never include neural parameters,
private learner state, random internals, archive hashes, or command receipts.

A job's `version` increments only for accepted user controls: creation, stop,
resume, and candidate save. Background progress increments `revision`, leaving
control versions stable. Use `expected_version` for commands and `revision` to
reject old progress snapshots. Receipts persist across restart; exact retries
return the current snapshot without executing twice. Candidate-save retries
return the originally published bot card. Missing jobs return 404, stale or reused
commands 409, invalid settings/control states 400. Failed jobs retain completed
records and diagnostics and cannot resume. Stopped/interrupted jobs can resume.

Snapshots contain lifecycle state, counters, config, active wall seconds, completed
generation history, last evaluated candidates, publications, and current completed
game count/budget. Evaluation snapshots additionally contain frozen model IDs,
config, completed score prefix, and aggregate stats only after full completion.
Private population/checkpoint internals and command receipts are not exposed.
List routes return summaries with history/candidates/publications/score arrays
empty; GET retrieves details. Generation numbers are 1-based archive indices;
candidate IDs are stable lineage identifiers, not bot IDs.
Neural list summaries likewise omit the `td_history` detail array.

Both event routes send `run.progress`, `checkpoint.saved`, `run.completed`, or
`run.failed` with `{run_id,kind,state,version,revision,generation}`. Progress is
coalesced to at most twice per second, with the initial state sent immediately
and 15-second keepalives. Event history is not replayed; reconnect/open/visibility
changes fetch the authoritative snapshot. Job streams share the existing limit
of 64 SSE connections and do not carry full game trajectories.

Training checkpoints live at `runs/<id>/checkpoint.json`, immutable generations
at `runs/<id>/generations/NNNN.json`, and evaluation checkpoints at
`evaluations/<id>/checkpoint.json`. The server and offline CLI share the same
exclusive data-directory owner and job manager. Training never modifies published
models or human sessions. A saved candidate is playable while another job runs.
Neural archives use the same directory, one immutable parameter/metric snapshot
per game. Neural stop/shutdown commits at a game boundary; crash recovery reruns
only uncommitted game work and verifies any already-created archive byte-for-byte.

## Watching training matches

`watched_game` is an optional small notice in training snapshots: key, generation,
zero-based game index within the generation, candidate/opponent IDs, candidate
color, turn count, and status. It identifies one actual evaluated match from the
most recently committed worker batch. No additional exhibition game is created.

For neural self-play, `generation` is the committed game number and `game_index`
is its zero-based index in the run. Both replay participant IDs are
`<algorithm>/self-play`: this is the live sequential learner for both colors,
not a match between frozen candidates. The recorded outcome/turn count agrees
with that game's TD metric; reconstructed positions and view controls are unchanged.

`GET /api/training/runs/{id}/watch` returns that notice together with `replay` and
`positions`. Positions come from Go replay validation: item 0 is the opening
position and item N follows full turn N. The result belongs to the same game
used for fitness and work counters. This read performs no inference or mutation.
A missing job, evaluation job, or run with no recorded sample returns 404.
The latest sample can advance between reading a notice and fetching its replay;
the endpoint always returns the currently retained sample with its own key.

Only one replay is retained per training checkpoint, under optional `watch_replay`.
The existing SHA-256 envelope protects it; startup checks match identity, schedule,
ruleset, full replay legality, and hashes. Format-1 checkpoints without viewer data
remain loadable. Existing completed runs without a recorded replay show an empty
preview. Resuming an older incomplete run records future samples normally.

The browser retains the playing game and at most one newer sample. Fetches are
coalesced to one in flight. Follow training advances to the newest available game
when the current replay finishes; Show latest game switches immediately. Pause,
speed, turn controls, and hidden-tab behavior affect only browser playback. SSE
remains small and throttled; slow/disconnected viewers do not backpressure the
scheduler. Playback cursors are transient and restart from the opening position
when the page reloads. Watching preserves the training budget, RNGs, scores,
selection, control versions, and work counters.
