import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { BudgetChart } from "./Research";
import type { ResearchRun } from "./research-data";

test("budget charts retain every seed and use measured selection-inclusive work", () => {
  const runs: ResearchRun[] = [11, 12].map((seed, i) => ({
    algorithm: "hybrid",
    seed,
    bot_id: "model",
    counters: {
      games: 40,
      completed_games: 40,
      truncated_games: 0,
      decisions: 300 + i * 150,
      forward_evaluations: 700,
      mutations: 0,
      crossovers: 0,
      updates: 100,
    },
    training_games: 8,
    selection_games: 32,
    wall_seconds: 1.25 + i,
    development_seconds: 0.5,
    development_counters: {
      games: 2,
      completed_games: 2,
      truncated_games: 0,
      decisions: 50,
      forward_evaluations: 90,
      mutations: 0,
      crossovers: 0,
    },
    estimate: {
      training_seeds: 1,
      pairs: 1,
      games: 2,
      win_rate: 0.5,
      mean_points: 0,
      mars_win_rate: 0,
      win_interval: { low: 0.5, high: 0.5 },
      point_interval: { low: 0, high: 0 },
      degenerate: true,
    },
  }));
  const decisions = renderToStaticMarkup(
    <BudgetChart runs={runs} axis="decisions" />,
  );
  const time = renderToStaticMarkup(<BudgetChart runs={runs} axis="time" />);
  expect(decisions).toContain('data-seed="11"');
  expect(decisions).toContain('data-seed="12"');
  expect(decisions).toContain('data-budget="450"');
  expect(time).toContain('data-budget="2.25"');
  expect(time).toContain("Measured training + selection time");
});
