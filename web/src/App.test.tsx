import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { App } from "./App";

test("play starts with a board and keeps training and settings separate", () => {
  const html = renderToStaticMarkup(<App />);
  expect(html).toContain("Main navigation");
  expect(html).toContain("Settings");
  expect(html).toContain("Loading opponents");
  expect(html).toContain("Start game");
  expect(html).toContain("Open Training");
  expect(html).toContain("Long nardy board");
  expect(html).not.toContain("THE LIBRARY");
  expect(html).toContain("long-nardy-fnr2026-nocube-v1");
});
