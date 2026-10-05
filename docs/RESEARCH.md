# Research contracts

## Statistics

`seed-stratified-pair-percentile-v1` accepts completed, ordered pairs: the target
plays White then Black against the same frozen opponent with the same dice seed.
A pair is the sampling unit; its two correlated games are never independently
resampled. Missing sides, duplicate opponent/seed pairs, changed schedules and
truncations have no estimate.

The estimator gives each training seed and opponent equal weight. A bootstrap
replicate samples training-seed clusters with replacement, then samples dice pairs
within each selected seed/opponent stratum. Method differences use the same
resampling indices and require matching schedules. Reports retain the bootstrap
seed, resample count and confidence level. Percentile bounds use linear
interpolation at `(n - 1) * quantile`. This is a transparent descriptive estimate,
not a guarantee of nominal coverage with a few seeds or degenerate samples.

The default is five independent training seeds, 2000 resamples and a two-sided
95% interval. Record every run, not only the best seed. Report win rate, signed
mean points per physical game and Mars win rate. A method difference is A minus B
on the shared opponent schedule; it is not a head-to-head probability.

The paired resampling and explicit percentile method follow the
[SciPy bootstrap reference](https://docs.scipy.org/doc/scipy/reference/generated/scipy.stats.bootstrap.html).
The need to show uncertainty across independent training runs is discussed in
[Deep RL at the Edge of the Statistical Precipice](https://proceedings.neurips.cc/paper/2021/hash/f514cec81cb148559cf475e7426eed5e-Abstract.html).
No SciPy dependency is required by the application.

## Champion confirmation

`single-heldout-incumbent-v1` declares the incumbent, held-out schedule, pair quota,
minimum independent pairs, bootstrap and required margin before the first final
game. Select one frozen candidate using development data only. Run one final batch
against one frozen incumbent. Confirmation requires the lower confidence bound
above `0.5 + minimum_margin`, enough independent pairs and a nondegenerate
bootstrap. Otherwise the model retains candidate status. The default minimum is
100 pairs; the configurable minimum cannot be below 30. Bounded smoke deliberately
uses fewer pairs and cannot confirm a champion.

Confirmation records a local result, without modifying model files or replacing
a global champion. Repeatedly choosing candidates using the final results turns
those results into development data; they cannot supply independent confirmation.
