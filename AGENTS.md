# EvoNardy

Read `docs/PROGRESS.md` before choosing the next milestone. The original brief is
`evonardy-codex-plan/EVONARDY_PLAN.md`; the current iteration is M0–M1.

For game behavior, use `docs/RULES.md` and the public engine tests. Changes to game
behavior require a new ruleset version and compatibility checks. Go owns legality;
the frontend consumes legal continuations. Preserve immutable published models,
separate dice randomness from agent randomness, and keep inference independent of
training. Record actual checks and incomplete work in `docs/PROGRESS.md`.

Use `make test`, `make smoke`, `make build`, and `make bench` once their prerequisites
are installed. Keep long training and publishing outside ordinary verification.
