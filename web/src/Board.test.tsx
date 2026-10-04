import { renderToStaticMarkup } from "react-dom/server";
import { expect, test } from "vitest";
import { Board } from "./Board";
import type { Position } from "./api";

test("board exposes server-approved points to keyboards and labels actual stacks", () => {
  const checkers = [Array<number>(24).fill(0), Array<number>(24).fill(0)];
  checkers[0][0] = 15;
  checkers[1][12] = 15;
  const p: Position = {
    ruleset: "long-nardy-fnr2026-nocube-v1",
    checkers,
    borne_off: [0, 0],
    turn: 0,
    starter: 0,
    first_done: [false, false],
  };
  const html = renderToStaticMarkup(
    <Board
      position={p}
      next={[{ from: 0, to: 3, die: 3 }]}
      selected={null}
      disabled={false}
      onPoint={() => {}}
    />,
  );
  expect(html).toContain(
    'aria-label="Point 1, 15 White checkers, select checker"',
  );
  expect(html).toContain('tabindex="0"');
  expect(html).toContain("Point 13, 15 Black checkers");
  expect(html).toContain("White off: 0");
});
