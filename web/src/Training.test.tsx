import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { FitnessChart, Jobs } from "./Training";

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
