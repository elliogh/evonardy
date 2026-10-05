# EvoNardy product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users and purpose

Browser players who want to play long nardy against built-in or locally trained
bots, and experiment with training their own opponents. Desktop browser use is
the primary target confirmed for the play-first redesign.

## Operating context

The application runs locally with a Go backend and React/TypeScript frontend.
Training means training bot models, not lessons or automated coaching for people.
The UI and repository content are English; user discussions can be Russian.

## Capabilities and constraints

- Long nardy uses `long-nardy-fnr2026-nocube-v1`, without doubling, clocks,
  accounts, betting, or online matchmaking. Go owns legal continuations.
- Built-in Random and Heuristic bots are available alongside saved immutable
  model snapshots. Measured evaluations do not imply named difficulty levels.
- Games persist dice, drafts and history on the server. Refresh and navigation
  must preserve the game; existing checker selection and turn controls remain.
- GA-linear, GA-MLP, TD(0), TD(lambda), Hybrid, independent evaluation and
  multi-seed research are existing capabilities.

## Product principles

- Playing is the first task. A first-time player can start against Heuristic
  without visiting the bot library; later choices are remembered.
- Returning players can continue unfinished games before starting another one.
- Play, Training and Settings are separate destinations. Model management,
  training runs and comparison belong in Training.
- Preserve real data and existing game behavior; never invent training results
  or silently switch the opponent when a new model is published.

## Evidence on hand

`docs/RULES.md`, `docs/TRAINING.md`, the implementation brief, the public game
engine tests, and the existing API and browser workflows describe the product.
