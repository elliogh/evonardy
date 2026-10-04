import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { App } from "./App";

test("library starts with real-data loading and states the current product boundary", () => {
  const html = renderToStaticMarkup(<App />);
  expect(html).toContain("My bots");
  expect(html).toContain("Loading your library");
  expect(html).toContain("Start training");
  expect(html).toContain("fresh dice seeds");
  expect(html).toContain("long-nardy-fnr2026-nocube-v1");
});
