# Local strength study — October 5, 2026

[Issue #48](https://github.com/elliogh/evonardy/issues/48) · [PR #49](https://github.com/elliogh/evonardy/pull/49)

## Decision

Retain **ev1**. It ranked first in the completed seven-policy development league
with 916 wins in 1200 games (76.3%). The best newly trained policy was
**GA-linear seed 1001**, with 868/1200 wins (72.3%). Install this policy separately
as **Candidate**; no new policy earned independent confirmation.

The incumbent won selection, so the predeclared protocol skipped the 500-pair
confirmation instead of testing the incumbent against itself. No final data was
used to choose another candidate. The direct development matchup was ev1
107/200 wins (53.5%, paired 95% interval 48.0–59.0%) against the best new policy.
That interval crosses 50%; this study does not establish a head-to-head superiority
claim. Against Heuristic, ev1 scored 118/200 (59.0%, 53.0–65.0%) in the league.
League results are selection data, not independent champion certification.

## Recorded protocol and actual scope

Study `6b592dd54ef4fb3192b67de145285030` ran on darwin/amd64, go1.25.5, using clean
revision `1b7f3fbb0f572d107d56b57307a2e48e45c559b9`. Actual elapsed campaign time was **112.7
minutes**, including storage and evaluation, under the approved two-hour limit.
Training admission stopped at 105 minutes; the deadline was never extended.

Seeds were 1001–1005. Each method/seed had 4096 physical training/selection games:

| Method | Configuration |
|---|---|
| GA-linear | Population 64, 8 generations, 2 pairs per opponent; initial sigma 0.4, mutation sigma 0.15 |
| GA-MLP | Population 16, 32 generations, 2 pairs per opponent; initial sigma 1, mutation sigma 0.02 |
| TD(0) | 4096 sequential self-play games, alpha 0.001, epsilon 0.05 |
| TD(lambda) | Same TD budget, lambda 0.7 |
| Hybrid | 8 participants, 8 rounds, 56 learning games plus selection per round; alpha 0.001, epsilon 0.05, lambda 0.7 |

Population methods used two workers; TD remained sequential. The turn guard was
1200. Published inference had zero exploration and stable tie breaking.
GA-linear used the nine-feature evaluator; neural methods used the shared
56→32→1 network. Hyperparameters were declared before execution, without tuning
on the observed final outcomes. Equal games do not imply equal compute, and this
comparison includes representation and configuration differences.

Every completed run received 50 swapped-side dice pairs each against frozen
Heuristic and ev1. Random received another 50 pairs as a separate diagnostic and
never determined selection. Training-seed indices shared development dice roots;
stage-specific roots were domain-separated by study ID. One finalist per method
was chosen on development, followed by 100 pairs per league matchup (21 matches).
Intervals used 2000 hierarchical pair-bootstrap resamples, seed 2026, confidence
95%. Exact requests, expanded seeds, roots, immutable manifests and raw game
scores remain in the local JSON report and durable job checkpoints.

**24 of 25 declared runs completed.** TD(lambda) seed 1005 was not admitted after
the cutoff. Seeds 1001–1004 remain visible individually; no aggregate method
estimate or paired method difference is assigned to that incomplete cohort.
Its best completed policy could participate in descriptive development selection.

## Full-cohort development results

These rates give equal weight to Heuristic and ev1 and to the five training
seeds. They describe the configured pipelines at this budget.

| Method | Training seeds | Win rate | 95% interval |
|---|---:|---:|---:|
| ga-linear | 5 | 41.8% | 36.4–47.2% |
| ga-mlp | 5 | 7.8% | 4.5–11.2% |
| td0 | 5 | 13.1% | 9.3–17.0% |
| hybrid | 5 | 7.8% | 5.6–10.0% |
| td-lambda | 4 of 5 | Incomplete cohort | No aggregate estimate |

GA-linear was the most promising new pipeline in this bounded study. Neural
methods were weak at these settings and budget; this does not imply they cannot
improve with longer training, different hyperparameters or a stronger curriculum.
The incumbent was historical and had a larger earlier training budget, so it is
a practical replacement target, not an equal-budget algorithm baseline.

## Descriptive league

Each policy played every other finalist in 200 games, with swapped sides.

| Policy | Wins/games | Win rate |
|---|---:|---:|
| ev1 | 916/1200 | 76.3% |
| Heuristic | 892/1200 | 74.3% |
| ga-linear seed 1001 | 868/1200 | 72.3% |
| td0 seed 1002 | 495/1200 | 41.2% |
| td-lambda seed 1004 | 381/1200 | 31.8% |
| hybrid seed 1002 | 335/1200 | 27.9% |
| ga-mlp seed 1002 | 313/1200 | 26.1% |

## Work and retained artifacts

All **109704 physical games completed; zero truncated games**. Actual work was
10065461 decisions, 171263938 forward evaluations, 5018961 TD updates,
4047645 parameter mutations and 1995 crossovers across 93 durable jobs.
Of the physical games, 98304 were training/selection, 7200 post-training checks,
and 4200 league games. The report retains actual trainer/evaluator call seconds
separately from campaign time, which also charges persistence and queue overhead.

- Original full user-test backup: `local-artifacts/test-data-20261005T154739Z/`,
  11199 verified files, 469588872 payload bytes, with checksums and restore steps.
- Study and raw jobs: `local-artifacts/strength-study-48/`.
- Original report JSON SHA-256:
  `d84b3789e6ac2db9efc780856a4ea8401078860030f373fac51698d52522c8e5`.
- Original executable SHA-256:
  `96a3d087ed647f1ad262bc3c0fb58fda1fbd54b7810959dc7b6885c7cc60a415`.
- Retained incumbent ID: `6625c585b0cfaf62f1b5745d2f1cd45d25906acf4f31c8903cabc6b79fcb2dcb`.
- Separate candidate ID: `1e3e6b795572a95fe20a8a44b3950344cd2e7ce09bdf25e44c975ea1ed35b482`.

Original reports and the executable are preserved separately before reporting
and installation refinements. Immutable model and manifest bytes are unchanged;
Candidate is mutable library metadata. Backups, raw scores and models stay local
and ignored by Git. This document publishes measurements and identities only.
A new study ID makes new random schedules; it does not undo any prior inspection
or make reused final data independent.

## Methodology references and limits

- [Agarwal et al., NeurIPS 2021](https://proceedings.neurips.cc/paper/2021/file/f514cec81cb148559cf475e7426eed5e-Paper.pdf)
  motivates reporting independent runs and uncertainty instead of the best seed alone.
- [Cawley and Talbot, JMLR 2010](https://www.jmlr.org/papers/v11/cawley10a.html)
  motivates separating model selection from performance evaluation.
- [SciPy bootstrap reference](https://docs.scipy.org/doc/scipy/reference/generated/scipy.stats.bootstrap.html)
  documents resampling and interval choices. The project implements its recorded
  pair/seed estimator without a SciPy dependency.
- [Tesauro, 1995](https://bkgm.com/articles/tesauro/tdl.html)
  describes TD self-play in short backgammon; it supplies no strength guarantee
  for these long-nardy policies.

Five seeds provide limited evidence; percentile intervals do not guarantee
nominal coverage in small or degenerate samples. Nothing here establishes
universal playing strength, performance against human experts or external bots.
Further training is a separate, explicitly budgeted experiment. M7 and general
retention tooling remain separate roadmap tasks.
