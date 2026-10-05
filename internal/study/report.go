package study

import (
	"fmt"
	"strings"

	"evonardy/internal/library"
	"evonardy/internal/training"
)

func markdown(s State) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Local strength study\n\nStudy `%s` — **%s** (%s).\n\n", s.ID, s.Status, s.Phase)
	fmt.Fprintf(&b, "Started: %s. Updated: %s. Limit: %d seconds, including persistence and evaluation.\n\nExecution: %s, %s, revision `%s` (modified: %t).\n\n", s.StartedAt.Format("2006-01-02T15:04:05Z07:00"), s.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"), s.Config.TimeLimitSeconds, s.Platform, s.GoVersion, s.Revision, s.Modified)
	fmt.Fprintf(&b, "Selected policy: `%s`. Best newly trained candidate: `%s`.\n\n%s\n\n", s.SelectedID, s.BestNewID, s.Reason)
	b.WriteString("## Protocol\n\nDevelopment ranks policies by equal-weight win rate against the frozen incumbent and Heuristic. Random is a separate diagnostic. Each observation is a White/Black pair sharing dice randomness. Training seeds and stage-specific dice roots, exact configs, manifests and raw evaluation scores are retained in `report.json` and durable job checkpoints. The study ID domain-separates the recorded schedules.\n\nThe tournament is development data. One candidate is locked before one final batch against the incumbent. A replacement requires a nondegenerate 95% bootstrap lower bound above the predeclared margin. Final results are never used to pick another candidate. This is evidence about tested opponents, not universal playing strength.\n\n")
	b.WriteString("## Independent training cohorts\n\nOnly complete declared seed cohorts receive aggregate method estimates. Bootstrap intervals describe uncertainty; a small cohort does not guarantee nominal coverage.\n\n| Method | Seeds | Win rate | 95% win interval | Mean points |\n|---|---:|---:|---:|---:|\n")
	for _, m := range s.Methods {
		e := m.Estimate
		fmt.Fprintf(&b, "| %s | %d | %.1f%% | %.1f–%.1f%% | %.3f |\n", m.Algorithm, e.TrainingSeeds, e.WinRate*100, e.WinInterval.Low*100, e.WinInterval.High*100, e.MeanPoints)
	}
	b.WriteString("\n| Method A | Method B | A − B (percentage points) | 95% interval |\n|---|---|---:|---:|\n")
	for _, d := range s.Differences {
		fmt.Fprintf(&b, "| %s | %s | %.1f | %.1f–%.1f |\n", d.A, d.B, d.Estimate.WinRate*100, d.Estimate.WinInterval.Low*100, d.Estimate.WinInterval.High*100)
	}
	b.WriteString("\nMethod differences compare performance on the shared opponent schedule; they are not head-to-head win probabilities. An interval crossing zero does not establish a difference.\n")
	b.WriteString("\n## Every completed training run\n\n| Method | Seed | Development win rate | Heuristic | Incumbent | Random | Physical training/selection games | Trainer seconds | Model |\n|---|---:|---:|---:|---:|---:|---:|---:|---|\n")
	for _, r := range s.Runs {
		fmt.Fprintf(&b, "| %s | %d | %.1f%% | %s | %s | %s | %d | %.2f | `%s` |\n", r.Method, r.Seed, r.Estimate.WinRate*100, opponentRate(s, r.DevelopmentID, library.HeuristicID), opponentRate(s, r.DevelopmentID, s.IncumbentID), opponentRate(s, r.RandomID, library.RandomID), r.Counters.Games, r.TrainerSeconds, r.BotID)
	}
	b.WriteString("\n## Development tournament\n\nEqual numbers of swapped-side games against every other finalist. Standings are descriptive; per-match intervals are reported below. An unfinished tournament cannot lock a candidate.\n\n| Model | Games | Wins | Win rate | Signed points |\n|---|---:|---:|---:|---:|\n")
	for _, s := range s.Standings {
		rate := 0.
		if s.Games > 0 {
			rate = float64(s.Wins) / float64(s.Games)
		}
		fmt.Fprintf(&b, "| `%s` | %d | %d | %.1f%% | %d |\n", s.BotID, s.Games, s.Wins, rate*100, s.Points)
	}
	b.WriteString("\n| A | B | A win rate | 95% interval | Mean points |\n|---|---|---:|---:|---:|\n")
	for _, m := range s.Matches {
		fmt.Fprintf(&b, "| `%s` | `%s` | %.1f%% | %.1f–%.1f%% | %.3f |\n", m.A, m.B, m.Estimate.WinRate*100, m.Estimate.WinInterval.Low*100, m.Estimate.WinInterval.High*100, m.Estimate.MeanPoints)
	}
	b.WriteString("\n## Independent confirmation\n\n")
	if s.Verdict == nil {
		b.WriteString("No completed confirmation verdict. The incumbent remains selected.\n")
	} else {
		v := s.Verdict
		fmt.Fprintf(&b, "Status: **%s**. %s. Win rate %.1f%%, interval %.1f–%.1f%%, %d independent pairs.\n", v.Status, v.Reason, v.Estimate.WinRate*100, v.Estimate.WinInterval.Low*100, v.Estimate.WinInterval.High*100, v.Estimate.Pairs)
	}
	var counters training.Counters
	var seconds float64
	for _, w := range s.Work {
		counters.Games += w.Counters.Games
		counters.CompletedGames += w.Counters.CompletedGames
		counters.TruncatedGames += w.Counters.TruncatedGames
		counters.Decisions += w.Counters.Decisions
		counters.ForwardEvaluations += w.Counters.ForwardEvaluations
		counters.Updates += w.Counters.Updates
		counters.Mutations += w.Counters.Mutations
		counters.Crossovers += w.Counters.Crossovers
		seconds += w.WallSeconds
	}
	fmt.Fprintf(&b, "\n## Actual work\n\n%d physical games: %d complete, %d truncated; %d decisions, %d forward evaluations, %d updates, %d mutations and %d crossovers. Measured trainer/evaluator calls: %.2f seconds; this excludes persistence, queue time and reporting. Equal game budgets do not imply equal compute.\n\nCompleted training runs: %d of %d declared. Every durable job, including interrupted work, is listed in the JSON report.\n", counters.Games, counters.CompletedGames, counters.TruncatedGames, counters.Decisions, counters.ForwardEvaluations, counters.Updates, counters.Mutations, counters.Crossovers, seconds, len(s.Runs), len(s.Config.Seeds)*len(s.Config.Methods))
	b.WriteString("\n## Methodology sources\n\n- [Deep RL at the Edge of the Statistical Precipice](https://proceedings.neurips.cc/paper/2021/file/f514cec81cb148559cf475e7426eed5e-Paper.pdf): report independent runs and uncertainty rather than a best seed alone.\n- [Cawley and Talbot, JMLR 2010](https://www.jmlr.org/papers/v11/cawley10a.html): keep model selection separate from final performance evaluation.\n- [Tesauro, 1995](https://bkgm.com/articles/tesauro/tdl.html): self-play TD results concern short backgammon; they do not establish the strength of these long-nardy policies.\n")
	return b.String()
}

func opponentRate(s State, job, opponent string) string {
	for _, e := range s.Evidence {
		if e.JobID != job {
			continue
		}
		wins, games := 0, 0
		for _, score := range e.Evaluation.Scores {
			if score.OpponentID == opponent && score.Status == "completed" {
				games++
				if score.Outcome.Winner == score.Side {
					wins++
				}
			}
		}
		if games > 0 {
			return fmt.Sprintf("%.1f%%", 100*float64(wins)/float64(games))
		}
	}
	return "unavailable"
}
