# Baseline features

Version `long-nardy-features-v1`, nine float64 values. Each feature is White's
contribution minus Black's, in [-1,1]. Heuristic maximizes for White and minimizes
for Black. Ties use stable LegalActions ordering; an immediate victory is selected
using Result before evaluation. Random samples unique final positions uniformly,
except that an available immediate victory takes priority.

| Feature | Contribution for one color | Weight |
|---|---|---:|
| pip_advantage | −sum(count × (24-progress)) / 360 | 1 |
| head_advantage | −checkers on the head / 15 | 0.45 |
| home_advantage | checkers at progress 18..23 / 15 | 0.25 |
| off_advantage | checkers borne off / 15 | 2 |
| occupied_advantage | occupied points / 15 | 0.2 |
| block_advantage | longest run on the opponent's finite route / 15 | 0.25 |
| distribution_advantage | −sum(count²) / 225 | 0.15 |
| rear_advantage | −max(24-progress) / 24; empty board → 0 | 0.2 |
| mobility_advantage | pairs (occupied source point, die 1..6) with progress+die<24 and a landing unoccupied by the opponent / 90 | 0.15 |

Mobility is a cheap proxy: it ignores full-turn obligations, the head limit,
bearing off, and intermediate blocks. It is not a second legality generator.
Evaluating all rolls using full-turn generation is not implemented or enabled
in this baseline. Distribution measures stack concentration rather than a
penalty for a single checker. Weights are an initial heuristic, and its strength
is unproven. Features and weights are not trained during ordinary play.
GA-linear arrives in M3 as a separate model with a manifest.
