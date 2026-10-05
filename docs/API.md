# Local API and persistence — M2

The server binds to loopback only. HTTP carries commands and authoritative
snapshots; SSE carries small change notifications. Frontend types are in
`web/src/api.ts`. Game behavior remains `long-nardy-fnr2026-nocube-v1`.

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

Format version 1 supports linear float64 weights (9 inputs → 1 value) and the
uniform random baseline. Linear inference uses stable first-action ties,
White value perspective, win objective, and zero exploration. Random is itself
a stochastic strategy, with a separate per-turn agent source. The loader checks
format/rules/features, shapes, weight count, raw-file SHA-256, finite weights
within ±1000000, and timestamps. Manifest/model files are limited to 64 KiB each;
metadata is limited to 1 MiB and 2048 rename receipts. Incompatible models remain
visible with their reason and cannot play. No model is labelled trained or
evaluated in M2. The data directory remains exclusively locked by one process.
