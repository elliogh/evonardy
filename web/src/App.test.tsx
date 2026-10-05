import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { App } from "./App";

test("scaffold makes unavailable functionality explicit", () => {
  const html = renderToStaticMarkup(<App />);
  expect(html).toContain("not available yet");
  expect(html).toContain("long-nardy-fnr2026-nocube-v1");
  expect(html).not.toContain("<button");
});
