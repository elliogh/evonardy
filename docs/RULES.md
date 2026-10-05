# EvoNardy rules

## Profile and source

`long-nardy-fnr2026-nocube-v1` implements long nardy, based on FNR rules dated
September 14, 2026 (order no. 874), articles 19, 20, 27, and 28:
https://sportnardy.ru/images/NARDI/Pravila_Nardy_2026.pdf.
The PDF was downloaded and the relevant articles checked during implementation.
This is a local game profile without a doubling cube, tournament clocks, penalties,
match scoring, refereeing, or physical dice procedures. There is no hitting,
bar for hit checkers, or triple-win backgammon result. A single opposing checker closes
a point completely; friendly checkers may be stacked.

SHA-256 of the verified PDF:
`0654e2618b5bdc261e7dc678cdfbd9d3cb8063f300ae1b4178959f7ac00d988b`.

Article 28.3 is not precise enough to directly specify an overshoot bearing-off
algorithm. The project **explicitly clarifies** it according to the brief:
an overshooting roll removes only a farthest remaining checker. This is the
project's formalization rather than a verbatim formula in the PDF. No discovered
conflict with the precise provisions of articles 20.3, 20.5, 27.2, and 27.5 has
been silently replaced with another variant.

## Coordinates and state

Global points `0..23` increase counterclockwise on the future board.
White's and Black's heads are `0/12`. For a player:

```
progress = (point - head + 24) % 24
point = (head + progress) % 24
home = progress 18..23
off = 24 (a separate counter, NOT a board point)
```

A checker traverses the finite route progress `0..23` and then bears off.
It never returns to the head. For example, Black moves from global 23 to global 1
on a roll of 2 (progress 11 → 13). A position is a copyable value: checker arrays,
off counters, side to move, starting player, first-turn completion flags, and ruleset.
Position validation requires 15 checkers per color including off, no overlapping
colors, valid counters/players, and no prohibited blocks. It does not prove that
an arbitrary test position is reachable from the opening.

## Opening, head, and movement

Each player rolls one die; ties are rerolled. The higher result starts and uses
that pair without another roll. All opening pairs are recorded in the replay.
Usually one checker may leave the head per full turn. Only the **second player's
first turn** allows two on 3–3, 4–4, or 6–6. Obstructions do not revoke that
permission, but landings on opposing checkers remain illegal. A pass completes
a player's first turn.

Each step uses one die, including intermediate landings. Both orders are
considered for different dice; doubles provide four steps. DFS checks the head,
landing, home, and blocking rules after EVERY step. Among nonterminal paths,
only those using the maximum number of dice are allowed. If globally only one
of two different dice can be used, choose the larger **actually available** die.
Exactly one pass is legal when no steps exist. A finished position has an empty
legal-action set rather than a new pass.

Victory is immediate: legal winning paths require no further steps, even if
another path would use more dice. If all paths have length one, the larger-die
rule still applies. For example, a last checker at progress 21 on 1–3 may bear
off directly on 3 or move on 1 and then bear off on 3. At progress 23 on 1–2,
when both paths have one step, use 2. At each full-turn boundary, first_done is
set and the turn switches, including in a terminal position. Off counters,
not the side-to-move field, determine the result.

## Six-point blocks

For each blocking color, scan the **opponent's finite route**, progress `0..23`.
A maximal consecutive run of occupied blocking points of length ≥6 is prohibited
when all 15 opposing checkers are strictly before its start. If at least one
opposing checker has passed the block or borne off, the block is allowed.
The end of the route is never joined to its beginning.

Examples of White blocking Black's route:

- Global `22,23,0,1,2,3` is one run at progress `10..15`.
  If all Black checkers are at progress 8, the block is prohibited despite crossing global zero.
- Global `10,11,12,13,14,15` is progress `22,23,0,1,2,3`.
  These are separate runs of length 2 and 4, not a six-point block.
- Filling the sixth point and then vacating it using another die is prohibited:
  a legal final position does not repair an illegal intermediate step.

## Home, bearing off, and results

Bearing off is available when all **remaining** checkers have progress ≥18.
This is checked after every step: the last checker outside home may enter and
bear off in the same full turn. Distance to exit is `24-progress`.

- An exact roll removes any checker on the corresponding point.
- An overshooting roll removes only a checker with minimum remaining progress.
  With checkers at progress 20 and 22, a roll of 6 removes 20, not 22.
- If only a checker at progress 22 remains, a roll of 6 removes it.
- With checkers at progress 17 and 23, a roll of 2 cannot remove 23: all must
  first reach home. On 1–6, the path 17 → 18 → off is legal, leaving the checker at 23.
- Moving within home remains an alternative to bearing off, subject to dice
  obligations. With checkers at progress 18 and 23, 1–6 may include 18 → 19
  followed by an overshooting bear-off from 19.

Victory at off=15 is immediate. If the opponent has borne off at least one
checker, the winner receives 1 point; otherwise a mars win awards 2 points.
Reaching a simulation turn limit yields `truncated`, not a win, loss, or draw.
Truncated games have no outcome or reward.

## Executable examples

`testdata/positions/rules.json` specifies layouts in each color's progress
coordinates, dice, and expected lengths/properties. Checkers missing from the
layout are counted as borne off to total 15. Positions unreachable from the
opening isolate particular rules. Fixtures run for both colors. Additional tests
cover blocks, invalid prefixes, alternative human paths, symmetry, an independent
slow oracle, and invariants of generated positions. Changes to these game
contracts require a new ruleset and replay/model compatibility checks.
