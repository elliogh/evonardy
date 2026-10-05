# EvoNardy

Read the open issues and milestones in `https://github.com/elliogh/evonardy`
before choosing work. Use `docs/WORKFLOW.md` for task tracking and completion.
The technical brief is `evonardy-codex-plan/EVONARDY_PLAN.md`.

For game behavior, use `docs/RULES.md` and the public engine tests. Changes to game
behavior require a new ruleset version and compatibility checks. Go owns legality;
the frontend consumes legal continuations. Preserve immutable published models,
separate dice randomness from agent randomness, and keep inference independent of
training. Record actual checks and incomplete work in the linked issue or PR.

Use `make test`, `make smoke`, `make build`, and `make bench` once their prerequisites
are installed. Keep long training and publishing outside ordinary verification.
