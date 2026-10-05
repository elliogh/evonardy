# Suggested repository instructions for EvoNardy

This is a template. Merge it with existing AGENTS.md after inspection; do not
replace another contributor's instructions automatically.

## Product

The project is long nardy on Go with a local React/TypeScript UI. Priorities:
correct play, training, immutable saved bots, and human play against loaded bots.
The full brief is EVONARDY_PLAN.md; game contracts are docs/RULES.md. Read open
issues and milestones in `https://github.com/elliogh/evonardy` before choosing
work; use docs/WORKFLOW.md for task tracking and completion.

## Stable constraints

Do not add standard-backgammon hitting, a bar, blot penalties, or a doubling cube.
Ruleset changes require versioning and tests. Go determines rules and legality;
the frontend does not duplicate them.

Agents cannot see future dice. Dice do not use exploration's PRNG. Publishing a
new model preserves old models. Inference never launches training. Reject
incompatible models explicitly.

The initial RL value is always from White's perspective; do not mix it with
negamax. GA-MLP, RL, and Hybrid use the same architecture and objective for
comparisons. GA-linear is a separate baseline. Do not invent playing strength,
speed, Elo, or a Hybrid victory.

## Workflow

Inspect the repository before changing it. Complete milestones in order and
record actual checks and incomplete work in the linked issue or PR. Keep Project
status current. Add regression tests for fixed defects. Complete checks
before marking work done. Report missing tools and failed checks. Do not run
large training jobs to verify the UI.

Bound worker counts, support context cancellation, and synchronize any shared
mutable model. Write data atomically. Do not push, publish, delete unrelated code,
or broadly replace instructions without a request.

## Checks

Once the corresponding modules exist: gofmt, go vet, go test, race detector;
frontend typecheck, tests, and build. Maintain exact commands and versions in
README/Makefile and report only checks that actually ran.

## Completion

Working behavior matters more than scaffolding. The first-release scenario is
train GA-linear → save → restart → select a bot → finish a game. Statistical
strength is a separate experiment, not a short-CI pass criterion.
