import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { FitnessChart, Jobs } from "./Training";
import { TDChart } from "./NeuralTraining";

test("training has bounded settings and no invented fitness", () => {
  const html = renderToStaticMarkup(<Jobs kind="training" />);
  expect(html).toContain("GA-linear");
  expect(html).toContain("Use smoke preset");
  expect(html).toContain("A truncated game fails the run");
  expect(renderToStaticMarkup(<FitnessChart history={[]} />)).toContain(
    "after a complete generation",
  );
});
test("fitness chart exposes the actual saved values accessibly", () => {
  const html = renderToStaticMarkup(
    <FitnessChart
      history={[
        {
          generation: 1,
          games: 16,
          best_fitness: 0.5,
          mean_fitness: 0.25,
          best_candidate_id: "g0-c1",
        },
      ]}
    />,
  );
  expect(html).toContain("Generation 1: best 0.500, mean 0.250");
  expect(html).toContain("Saved generation fitness");
});

test("TD charts report real error and truncation without invented results", () => {
  expect(renderToStaticMarkup(<TDChart history={[]} />)).toContain(
    "after the first game boundary",
  );
  const html = renderToStaticMarkup(
    <TDChart
      history={[
        {
          game: 1,
          status: "truncated",
          outcome: null,
          decisions: 8,
          forward_evaluations: 36,
          updates: 8,
          mean_abs_delta: 0.012345,
        },
      ]}
    />,
  );
  expect(html).toContain(
    "Game 1: mean absolute TD error 0.012345, 8 updates, truncated",
  );
  expect(html).toContain("use independent evaluation");
  expect(html).not.toContain("win probability");
});
