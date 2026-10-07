import { test, expect, type Page } from "@playwright/test";
import { spawn, type ChildProcess } from "node:child_process";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import type { Bot, Snapshot } from "../src/api";
import type { Job } from "../src/jobs";
import { botStepDelay } from "../src/botPlayback";

const root = fileURLToPath(new URL("../../", import.meta.url));
const baseURL = "http://127.0.0.1:18180";
// One roll frame and two frames per move, including all four moves of a double.
const playbackTimeout = 9 * botStepDelay + 5_000;

test.beforeEach(async ({ page }) => {
  await page.clock.install();
});
let dataDir: string;
let server: ChildProcess;
async function start() {
  let log = "";
  server = spawn(
    `${root}bin/evonardy`,
    [
      "serve",
      "--addr",
      "127.0.0.1:18180",
      "--data-dir",
      dataDir,
      "--web-dir",
      `${root}web/dist`,
    ],
    { cwd: root },
  );
  server.stdout?.on("data", (chunk) => {
    log += chunk;
  });
  server.stderr?.on("data", (chunk) => {
    log += chunk;
  });
  await expect
    .poll(async () => {
      if (server.exitCode !== null) throw new Error(log);
      try {
        return (await fetch(`${baseURL}/api/bots`)).status;
      } catch {
        return 0;
      }
    })
    .toBe(200);
}
async function stop() {
  if (server.exitCode !== null) return;
  const exited = new Promise<void>((resolve) =>
    server.once("exit", () => resolve()),
  );
  server.kill("SIGTERM");
  await exited;
}
test.beforeAll(async () => {
  dataDir = await mkdtemp(`${tmpdir()}/evonardy-e2e-`);
  await start();
});
test.afterAll(async () => {
  await stop();
  await rm(dataDir, { recursive: true, force: true });
});

test("watch real training games without pausing learning and recover the board after restart", async ({
  page,
}, info) => {
  const recorded = new Map<
    string,
    {
      key: string;
      positions: Snapshot["position"][];
      replay: { outcome: Snapshot["outcome"]; events: unknown[] };
    }
  >();
  page.on("response", async (response) => {
    if (response.url().endsWith("/watch") && response.ok()) {
      const x = await response.json();
      recorded.set(x.key, x);
    }
  });
  await page.goto("/#/training");
  await page
    .getByRole("button", { name: "Use smoke preset", exact: true })
    .click();
  await page
    .getByRole("spinbutton", { name: "Generations", exact: true })
    .fill("8");
  await page
    .getByRole("spinbutton", { name: "Pairs per opponent", exact: true })
    .fill("4");
  await page
    .getByRole("button", { name: "Start training", exact: true })
    .click();
  await expect(page).toHaveURL(/#\/training\//);
  const id = page.url().split("/training/")[1];
  const job = async (): Promise<Job> =>
    await (await page.request.get(`/api/training/runs/${id}`)).json();
  const viewer = page.locator(".watched-match");
  await expect(viewer).toBeVisible();
  await expect
    .poll(async () => Number(await viewer.getAttribute("data-frame")))
    .toBeGreaterThan(0);
  await page
    .getByRole("button", { name: "Pause playback", exact: true })
    .click();
  const slider = page.getByRole("slider", {
    name: "Training game turn",
    exact: true,
  });
  await slider.focus();
  await slider.press("Home");
  await expect(viewer).toHaveAttribute("data-frame", "0");
  await slider.press("ArrowRight");
  await expect(viewer).toHaveAttribute("data-frame", "1");
  const key = (await viewer.getAttribute("data-game-key"))!;
  await expect.poll(() => recorded.has(key)).toBe(true);
  const position = recorded.get(key)!.positions[1];
  for (let point = 0; point < 24; point++) {
    const count = position.checkers[0][point] + position.checkers[1][point];
    const player = position.checkers[0][point] > 0 ? "White" : "Black";
    await expect(viewer.locator(`[data-point="${point}"]`)).toHaveAttribute(
      "aria-label",
      `Point ${point + 1}, ${count ? `${count} ${player} checkers` : "empty"}`,
    );
  }
  const before = await job();
  expect(before.state).toBe("running");
  await expect
    .poll(async () => (await job()).counters.games)
    .toBeGreaterThan(before.counters.games);
  await expect(viewer).toHaveAttribute("data-frame", "1");
  await expect(viewer).toHaveAttribute("data-game-key", key);
  await page.getByRole("button", { name: "Next turn", exact: true }).click();
  await expect(viewer).toHaveAttribute("data-frame", "2");
  await page
    .getByRole("button", { name: "Previous turn", exact: true })
    .click();
  await expect(viewer).toHaveAttribute("data-frame", "1");
  await slider.focus();
  await slider.press("End");
  await expect(viewer).toHaveAttribute(
    "data-frame",
    String(recorded.get(key)!.replay.events.length),
  );
  const winner =
    recorded.get(key)!.replay.outcome!.winner === 0 ? "White" : "Black";
  await expect(viewer.locator(".match-result")).toContainText(`${winner} wins`);
  await page
    .getByRole("button", { name: "Stop and checkpoint", exact: true })
    .click();
  await expect(page.locator(".job-state")).toHaveAttribute(
    "data-state",
    "stopped",
  );
  const latest = await (
    await page.request.get(`/api/training/runs/${id}/watch`)
  ).json();
  await page
    .getByRole("button", { name: "Show latest game", exact: true })
    .click();
  await expect(viewer).toHaveAttribute("data-game-key", latest.key);
  await page.screenshot({
    path: info.outputPath("watch-desktop.png"),
    fullPage: true,
  });
  await stop();
  await start();
  await page.reload();
  await expect(viewer).toHaveAttribute("data-game-key", latest.key);
  expect(
    await (await page.request.get(`/api/training/runs/${id}/watch`)).json(),
  ).toEqual(latest);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: info.outputPath("watch-mobile.png"),
    fullPage: true,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});

async function snapshot(page: Page, id: string): Promise<Snapshot> {
  let current: Snapshot;
  const read = async () => {
    const response = await page.request.get(`/api/games/${id}`);
    expect(response.ok()).toBeTruthy();
    current = await response.json();
    return current;
  };
  if (page.url().endsWith(`#/games/${id}`)) {
    const status = page.locator(".turn-status");
    await expect
      .poll(
        async () => {
          if ((await status.getAttribute("data-playback")) === "playing") {
            // Advance each frame so React can render and schedule the next one.
            await page.clock.runFor(botStepDelay);
          }
          const latest = await read();
          return status.evaluate(
            (element, version) =>
              element.getAttribute("data-version") === String(version) &&
              element.getAttribute("data-playback") === "idle" &&
              /^(moving|finished)$/.test(element.getAttribute("data-phase") ?? ""),
            latest.version,
          );
        },
        { timeout: playbackTimeout, intervals: [50] },
      )
      .toBe(true);
    return current!;
  }
  return read();
}
async function rendered(page: Page, x: Snapshot) {
  await expect(page.locator(".turn-status")).toHaveAttribute(
    "data-version",
    String(x.version),
  );
}
async function buttonCommand(page: Page, id: string, name: string) {
  const before = await snapshot(page, id);
  await page.getByRole("button", { name, exact: true }).click();
  await expect
    .poll(async () => (await snapshot(page, id)).version, {
      timeout: playbackTimeout,
    })
    .toBeGreaterThan(before.version);
  const x = await snapshot(page, id);
  const confirms = name === "Confirm turn" || name === "Pass turn";
  expect(x.version).toBe(
    before.version + (confirms && x.phase !== "finished" ? 2 : 1),
  );
  await rendered(page, x);
  return x;
}
async function firstStep(page: Page, id: string) {
  const before = await snapshot(page, id);
  await rendered(page, before);
  const step = before.continuations.next[0];
  const source = page.locator(`[data-point="${step.from}"]`);
  await source.focus();
  await source.press("Enter");
  if (step.to === 24)
    await page.getByRole("button", { name: "Bear off", exact: true }).click();
  else await page.locator(`[data-point="${step.to}"]`).click();
  if (
    before.continuations.next.filter(
      (s) => s.from === step.from && s.to === step.to,
    ).length > 1
  )
    await page
      .getByRole("button", { name: `Move with ${step.die}`, exact: true })
      .click();
  await expect
    .poll(async () => (await snapshot(page, id)).version, {
      timeout: playbackTimeout,
    })
    .toBe(before.version + 1);
  const x = await snapshot(page, id);
  await rendered(page, x);
  return x;
}

test("save and rename a bot, recover a draft, and finish a real browser game", async ({
  page,
}, info) => {
  const errors: string[] = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto("/#/bots");
  await expect(
    page.getByRole("button", { name: "Play against Heuristic", exact: true }),
  ).toBeVisible();
  await page
    .locator(".bot-card")
    .filter({
      has: page.getByRole("heading", { name: "Heuristic", exact: true }),
    })
    .getByRole("button", { name: "Save snapshot", exact: true })
    .click();
  await page
    .getByRole("textbox", { name: "Snapshot name" })
    .fill("My baseline");
  await page.getByRole("button", { name: "Save bot", exact: true }).click();
  await page
    .getByRole("button", { name: "Rename My baseline", exact: true })
    .click();
  await page
    .getByRole("textbox", { name: "Snapshot name" })
    .fill("Opening study");
  await page.getByRole("button", { name: "Apply name", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Opening study", exact: true }),
  ).toBeVisible();
  await page.screenshot({
    path: info.outputPath("library-desktop.png"),
    fullPage: true,
  });
  await page
    .getByRole("button", { name: "Play against Heuristic", exact: true })
    .click();
  await expect(page).toHaveURL(/#\/games\//);
  const id = page.url().split("/games/")[1];
  let x = await snapshot(page, id);
  await rendered(page, x);
  x = await firstStep(page, id);
  const saved = x;
  await page.reload();
  await rendered(page, saved);
  expect(await snapshot(page, id)).toEqual(saved);
  await stop();
  await start();
  await page.reload();
  await rendered(page, saved);
  expect(await snapshot(page, id)).toEqual(saved);
  x = await buttonCommand(page, id, "Undo");
  expect(x.draft).toHaveLength(0);
  expect(x.dice).toEqual(saved.dice);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: info.outputPath("game-mobile.png"),
    fullPage: true,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page.screenshot({
    path: info.outputPath("game-desktop.png"),
    fullPage: true,
  });
  // Lose the first confirmation response after the real server commits it.
  // The browser must retry the same command, leaving one version and one bot turn.
  let loseResponse = true;
  await page.route("**/api/games/*/turn", async (route) => {
    if (loseResponse) {
      loseResponse = false;
      await route.fetch();
      await route.abort("failed");
    } else await route.continue();
  });
  let commands = 0;
  while (x.phase !== "finished" && commands++ < 1500) {
    while (!x.continuations.complete) x = await firstStep(page, id);
    x = await buttonCommand(
      page,
      id,
      x.draft.length ? "Confirm turn" : "Pass turn",
    );
  }
  expect(x.phase).toBe("finished");
  await expect(
    page.getByRole("link", { name: "Download replay" }),
  ).toBeVisible();
  const response = await page.request.get(`/api/replays/${id}`);
  expect(response.ok()).toBeTruthy();
  expect((await response.json()).status).toBe("completed");
  await page.goto("/#/bots");
  await expect(
    page.getByRole("heading", { name: "Opening study", exact: true }),
  ).toBeVisible();
  await page.goto("/");
  await page.locator(".recent-section summary").click();
  await expect(
    page.locator(`.recent-game[href="#/games/${id}"]`),
  ).toContainText("wins");
  await page.goto(`/#/games/${id}`);
  await page.getByRole("button", { name: "Rematch", exact: true }).click();
  await expect(page).not.toHaveURL(new RegExp(`${id}$`));
  const rematch = await snapshot(page, page.url().split("/games/")[1]);
  expect(rematch.bot_id).toBe(x.bot_id);
  expect(rematch.human).toBe(x.human);
  expect(rematch.id).not.toBe(id);
  expect(errors).toEqual([]);
});

test("a saved model plays as Black and another browser receives draft updates", async ({
  page,
  context,
}) => {
  const response = await page.request.post("/api/bots", {
    data: {
      command_id: crypto.randomUUID(),
      expected_version: 0,
      source_bot_id: "builtin/heuristic-v1",
      name: "Black study",
    },
  });
  expect(response.ok()).toBeTruthy();
  const bot: Bot = await response.json();
  await page.goto("/#/bots");
  const saved = page.locator(".bot-card").filter({
    has: page.getByRole("heading", { name: bot.name, exact: true }),
  });
  await expect(saved).toBeVisible();
  await page.goto("/#/settings");
  await page.getByRole("combobox", { name: "Your color" }).selectOption("1");
  await page.goto("/#/bots");
  await saved
    .getByRole("button", { name: `Play against ${bot.name}`, exact: true })
    .click();
  await expect(page).toHaveURL(/#\/games\//);
  const id = page.url().split("/games/")[1];
  let x = await snapshot(page, id);
  await rendered(page, x);
  expect(x.human).toBe(1);
  const observer = await context.newPage();
  await observer.clock.install();
  await observer.goto(`/#/games/${id}`);
  await rendered(observer, x);
  x = await firstStep(page, id);
  await rendered(observer, x);
  while (!x.continuations.complete) x = await firstStep(page, id);
  x = await buttonCommand(page, id, "Confirm turn");
  await rendered(observer, x);
  expect(x.history.filter((h) => h.actor === 1)).toHaveLength(1);
  await observer.close();
});

test("train, publish, independently evaluate, resume after restart, and play a frozen bot", async ({
  page,
  context,
}, info) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/#/training");
  await page
    .getByRole("button", { name: "Use smoke preset", exact: true })
    .click();
  await page
    .getByRole("textbox", { name: "Run name", exact: true })
    .fill("Browser GA smoke");
  const bodies: string[] = [];
  let lose = true;
  await page.route("**/api/training/runs", async (route) => {
    if (route.request().method() !== "POST") {
      await route.continue();
      return;
    }
    bodies.push(route.request().postData()!);
    if (lose) {
      lose = false;
      await route.fetch();
      await route.abort("failed");
    } else await route.continue();
  });
  await page
    .getByRole("button", { name: "Start training", exact: true })
    .click();
  await expect(page).toHaveURL(/#\/training\//);
  const id = page.url().split("/training/")[1];
  const job = async (runID = id): Promise<Job> => {
    const r = await page.request.get(`/api/training/runs/${runID}`);
    expect(r.ok()).toBeTruthy();
    return r.json();
  };
  await expect
    .poll(async () => (await job()).state, { timeout: 60000 })
    .toBe("completed");
  await expect(page.locator(".job-state")).toHaveAttribute(
    "data-state",
    "completed",
  );
  expect(bodies).toHaveLength(2);
  expect(bodies[0]).toBe(bodies[1]);
  const trained = await job();
  expect(trained.counters.games).toBe(32);
  expect(trained.counters.mutations).toBeGreaterThan(0);
  const candidate = trained.candidates.find(
    (c) =>
      JSON.stringify(c.weights) !==
      JSON.stringify([1, 0.45, 0.25, 2, 0.2, 0.25, 0.15, 0.2, 0.15]),
  )!;
  expect(candidate).toBeTruthy();
  await page
    .getByRole("combobox", { name: "Candidate", exact: true })
    .selectOption(candidate.id);
  await page
    .getByRole("textbox", { name: "Bot name", exact: true })
    .fill("M3 browser bot");
  await page
    .getByRole("button", { name: "Save candidate", exact: true })
    .click();
  await expect(
    page.getByRole("link", { name: "Evaluate this bot", exact: true }),
  ).toBeVisible();
  const bots: Bot[] = await (await page.request.get("/api/bots")).json();
  const bot = bots.find((b) => b.name === "M3 browser bot")!;
  expect(bot).toBeTruthy();
  const manifest = await (await page.request.get(`/api/bots/${bot.id}`)).json();
  await page.screenshot({
    path: info.outputPath("training-desktop.png"),
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: info.outputPath("training-mobile.png"),
    fullPage: true,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.setViewportSize({ width: 1280, height: 900 });
  await page
    .getByRole("link", { name: "Evaluate this bot", exact: true })
    .click();
  await page
    .getByRole("spinbutton", { name: "Pairs per opponent", exact: true })
    .fill("1");
  await page
    .getByRole("button", { name: "Start evaluation", exact: true })
    .click();
  await expect(page).toHaveURL(/#\/evaluations\//);
  const evaluationID = page.url().split("/evaluations/")[1];
  await expect
    .poll(
      async () =>
        (
          (await (
            await page.request.get(`/api/evaluations/${evaluationID}`)
          ).json()) as Job
        ).state,
      { timeout: 30000 },
    )
    .toBe("completed");
  await expect(
    page.getByText("4 games", { exact: false }).first(),
  ).toBeVisible();
  await expect(page.getByText("Win rate", { exact: true })).toBeVisible();
  await page.goto("/#/training");
  await page
    .getByRole("button", { name: "Use smoke preset", exact: true })
    .click();
  await page
    .getByRole("spinbutton", { name: "Generations", exact: true })
    .fill("3");
  await page
    .getByRole("spinbutton", { name: "Pairs per opponent", exact: true })
    .fill("2");
  await page
    .getByRole("button", { name: "Start training", exact: true })
    .click();
  await expect(page).toHaveURL(/#\/training\//);
  const other = page.url().split("/training/")[1];
  await expect
    .poll(async () => (await job(other)).counters.games)
    .toBeGreaterThan(0);
  const player = await context.newPage();
  await player.clock.install();
  await player.goto("/#/bots");
  await player
    .getByRole("button", { name: `Play against ${bot.name}`, exact: true })
    .click();
  await expect(player).toHaveURL(/#\/games\//);
  const gameID = player.url().split("/games/")[1];
  expect(["running", "queued"]).toContain((await job(other)).state);
  await page
    .getByRole("button", { name: "Stop and checkpoint", exact: true })
    .click();
  await expect(page.locator(".job-state")).toHaveAttribute(
    "data-state",
    "stopped",
  );
  const stopped = await job(other);
  expect(stopped.can_resume).toBe(true);
  expect(stopped.counters.games).toBeGreaterThan(0);
  expect(stopped.counters.games).toBeLessThan(96);
  const game = await snapshot(player, gameID);
  await stop();
  await start();
  await page.reload();
  await player.reload();
  await rendered(player, game);
  expect(await snapshot(player, gameID)).toEqual(game);
  expect(await (await page.request.get(`/api/bots/${bot.id}`)).json()).toEqual(
    manifest,
  );
  await expect(
    page.getByRole("button", { name: "Resume run", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Resume run", exact: true }).click();
  let x = game;
  let commands = 0;
  while (x.phase !== "finished" && commands++ < 1500) {
    while (!x.continuations.complete) x = await firstStep(player, gameID);
    x = await buttonCommand(
      player,
      gameID,
      x.draft.length ? "Confirm turn" : "Pass turn",
    );
  }
  expect(x.phase).toBe("finished");
  expect(
    (await (await player.request.get(`/api/replays/${gameID}`)).json()).status,
  ).toBe("completed");
  await expect
    .poll(async () => (await job(other)).state, { timeout: 60000 })
    .toBe("completed");
  expect((await job(other)).counters.games).toBe(96);
  expect(await (await page.request.get(`/api/bots/${bot.id}`)).json()).toEqual(
    manifest,
  );
  await player.close();
  await page.goto("/#/bots");
  const card = page.locator(".bot-card").filter({
    has: page.getByRole("heading", { name: bot.name, exact: true }),
  });
  await expect(card).toContainText("4 games");
  expect(errors).toEqual([]);
});

for (const method of ["td0", "td-lambda"] as const) {
  const label = method === "td0" ? "TD(0)" : "TD(lambda)";
  test(`${label}: train, save, restart, evaluate, watch, and play a real neural model`, async ({
    page,
    context,
  }, info) => {
    const errors: string[] = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await page.goto("/#/training");
    await page
      .getByRole("combobox", { name: "Training method", exact: true })
      .selectOption(method);
    await page
      .getByRole("button", { name: "Use smoke preset", exact: true })
      .click();
    await expect(
      page.getByRole("spinbutton", { name: "Games", exact: true }),
    ).toHaveValue("4");
    await expect(
      page.getByRole("spinbutton", { name: "Population", exact: true }),
    ).toHaveCount(0);
    await expect(
      page.getByRole("spinbutton", { name: "Workers", exact: true }),
    ).toHaveCount(0);
    await page
      .getByRole("textbox", { name: "Run name", exact: true })
      .fill(`M4 ${method} smoke`);
    await page
      .getByRole("button", { name: "Start training", exact: true })
      .click();
    await expect(page).toHaveURL(/#\/training\//);
    const id = page.url().split("/training/")[1];
    const job = async (runID = id): Promise<Job> => {
      const response = await page.request.get(`/api/training/runs/${runID}`);
      expect(response.ok()).toBeTruthy();
      return response.json();
    };
    await expect
      .poll(async () => (await job()).state, { timeout: 60000 })
      .toBe("completed");
    await expect(page.locator(".job-state")).toHaveAttribute(
      "data-state",
      "completed",
    );
    const trained = await job();
    expect(trained.algorithm).toBe(
      method === "td0" ? "td-zero-v1" : "td-lambda-v1",
    );
    expect(trained.td_config?.lambda ?? 0).toBe(method === "td0" ? 0 : 0.7);
    expect(trained.counters.games).toBe(4);
    expect(trained.counters.completed_games).toBe(4);
    expect(trained.counters.updates).toBeGreaterThan(0);
    expect(trained.counters.updates).toBe(trained.counters.decisions);
    expect(trained.counters.mutations).toBe(0);
    expect(trained.td_history).toHaveLength(4);
    await expect(page.locator("[data-updates]")).toHaveAttribute(
      "data-updates",
      String(trained.counters.updates),
    );
    await expect(
      page.getByRole("img", {
        name: "Mean absolute TD error per saved game",
        exact: true,
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: "Generation fitness", exact: true }),
    ).toHaveCount(0);

    const viewer = page.locator(".watched-match");
    await expect(viewer).toBeVisible();
    await expect(viewer).toContainText(`${label} self-play`);
    await page
      .getByRole("button", { name: "Pause playback", exact: true })
      .click();
    const watched = await (
      await page.request.get(`/api/training/runs/${id}/watch`)
    ).json();
    const latest = page.getByRole("button", {
      name: "Show latest game",
      exact: true,
    });
    if (await latest.isEnabled()) await latest.click();
    await expect(viewer).toHaveAttribute("data-game-key", watched.key);
    const slider = page.getByRole("slider", {
      name: "Training game turn",
      exact: true,
    });
    await slider.focus();
    await slider.press("Home");
    await slider.press("ArrowRight");
    await expect(viewer).toHaveAttribute("data-frame", "1");
    const position: Snapshot["position"] = watched.positions[1];
    for (let point = 0; point < 24; point++) {
      const count = position.checkers[0][point] + position.checkers[1][point];
      const player = position.checkers[0][point] > 0 ? "White" : "Black";
      await expect(viewer.locator(`[data-point="${point}"]`)).toHaveAttribute(
        "aria-label",
        `Point ${point + 1}, ${count ? `${count} ${player} checkers` : "empty"}`,
      );
    }
    await page
      .getByRole("textbox", { name: "Bot name", exact: true })
      .fill(`M4 ${method} bot`);
    await page
      .getByRole("button", { name: "Save neural snapshot", exact: true })
      .click();
    await expect(
      page.getByRole("link", { name: "Evaluate this bot", exact: true }),
    ).toBeVisible();
    const bots: Bot[] = await (await page.request.get("/api/bots")).json();
    const bot = bots.find((b) => b.name === `M4 ${method} bot`)!;
    expect(bot?.kind).toBe("tanh-56-32-1-v1");
    const detail = await (await page.request.get(`/api/bots/${bot.id}`)).json();
    expect(detail.manifest.architecture).toEqual([56, 32, 1]);
    const modelPath = `${dataDir}/bots/${bot.id}/model.json`;
    const modelBefore = await readFile(modelPath, "utf8");
    const weights = JSON.parse(modelBefore).weights;
    expect(weights).toHaveLength(1857);
    const checkpoint = JSON.parse(
      await readFile(`${dataDir}/runs/${id}/checkpoint.json`, "utf8"),
    );
    expect(weights).toEqual(checkpoint.record.td_training.parameters);
    await page.screenshot({
      path: info.outputPath(`${method}-desktop.png`),
      fullPage: true,
    });
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({
      path: info.outputPath(`${method}-mobile.png`),
      fullPage: true,
    });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    await page.setViewportSize({ width: 1280, height: 900 });

    await stop();
    await start();
    await page.reload();
    await expect(viewer).toHaveAttribute("data-game-key", watched.key);
    expect((await job()).td_history).toEqual(trained.td_history);
    expect((await job()).counters).toEqual(trained.counters);
    expect(
      await (await page.request.get(`/api/bots/${bot.id}`)).json(),
    ).toEqual(detail);
    await page.getByRole("link", { name: "My bots", exact: true }).click();
    const card = page.locator(".bot-card").filter({
      has: page.getByRole("heading", { name: bot.name, exact: true }),
    });
    await expect(card).toContainText("A frozen neural strategy");
    await card
      .getByRole("link", { name: `Evaluate ${bot.name}`, exact: true })
      .click();
    await page
      .getByRole("spinbutton", { name: "Pairs per opponent", exact: true })
      .fill("1");
    await page
      .getByRole("button", { name: "Start evaluation", exact: true })
      .click();
    await expect(page).toHaveURL(/#\/evaluations\//);
    const evaluationID = page.url().split("/evaluations/")[1];
    const evaluation = async (): Promise<Job> =>
      (await page.request.get(`/api/evaluations/${evaluationID}`)).json();
    await expect
      .poll(async () => (await evaluation()).state, { timeout: 60000 })
      .toBe("completed");
    expect((await evaluation()).evaluation?.stats?.games).toBe(4);
    expect((await evaluation()).counters.updates ?? 0).toBe(0);
    await expect(page.getByText("Win rate", { exact: true })).toBeVisible();

    // Play the immutable model while another learner runs; pause the viewer only.
    // Stop early so the larger budget is a guard, not a long verification run.
    await page.goto("/#/training");
    await page
      .getByRole("combobox", { name: "Training method", exact: true })
      .selectOption(method);
    await page
      .getByRole("button", { name: "Use smoke preset", exact: true })
      .click();
    await page
      .getByRole("spinbutton", { name: "Games", exact: true })
      .fill("128");
    await page
      .getByRole("button", { name: "Start training", exact: true })
      .click();
    await expect(page).toHaveURL(/#\/training\//);
    const other = page.url().split("/training/")[1];
    await expect(viewer).toBeVisible();
    await page
      .getByRole("button", { name: "Pause playback", exact: true })
      .click();
    const frame = await viewer.getAttribute("data-frame");
    const key = await viewer.getAttribute("data-game-key");
    const before = await job(other);
    expect(before.state).toBe("running");
    await expect
      .poll(async () => (await job(other)).counters.games)
      .toBeGreaterThan(before.counters.games);
    await expect(viewer).toHaveAttribute("data-frame", frame!);
    await expect(viewer).toHaveAttribute("data-game-key", key!);
    const player = await context.newPage();
    await player.clock.install();
    player.on("pageerror", (e) => errors.push(e.message));
    await player.goto("/#/bots");
    await player
      .getByRole("button", { name: `Play against ${bot.name}`, exact: true })
      .click();
    await expect(player).toHaveURL(/#\/games\//);
    const gameID = player.url().split("/games/")[1];
    expect((await job(other)).state).toBe("running");
    await page
      .getByRole("button", { name: "Stop and checkpoint", exact: true })
      .click();
    await expect(page.locator(".job-state")).toHaveAttribute(
      "data-state",
      "stopped",
    );
    const stopped = await job(other);
    expect(stopped.counters.games).toBeGreaterThan(0);
    expect(stopped.counters.games).toBeLessThan(128);
    const game = await snapshot(player, gameID);
    await stop();
    await start();
    await page.reload();
    await player.reload();
    await rendered(player, game);
    expect(await snapshot(player, gameID)).toEqual(game);
    expect((await job(other)).counters).toEqual(stopped.counters);
    await page.getByRole("button", { name: "Resume run", exact: true }).click();
    await expect
      .poll(async () => (await job(other)).counters.games)
      .toBeGreaterThan(stopped.counters.games);
    await page
      .getByRole("button", { name: "Stop and checkpoint", exact: true })
      .click();
    await expect(page.locator(".job-state")).toHaveAttribute(
      "data-state",
      "stopped",
    );
    let x = game;
    let commands = 0;
    while (x.phase !== "finished" && commands++ < 1500) {
      while (!x.continuations.complete) x = await firstStep(player, gameID);
      x = await buttonCommand(
        player,
        gameID,
        x.draft.length ? "Confirm turn" : "Pass turn",
      );
    }
    expect(x.phase).toBe("finished");
    expect(
      (await (await player.request.get(`/api/replays/${gameID}`)).json())
        .status,
    ).toBe("completed");
    expect(await readFile(modelPath, "utf8")).toBe(modelBefore);
    expect(
      await (await page.request.get(`/api/bots/${bot.id}`)).json(),
    ).toEqual(detail);
    await player.close();
    await page.getByRole("link", { name: "My bots", exact: true }).click();
    await expect(card).toContainText("4 games");
    expect(errors).toEqual([]);
  });
}

for (const method of ["ga-mlp", "hybrid"] as const) {
  test(`${method}: publish archived neural candidates, recover population and play frozen weights`, async ({
    page,
    context,
  }, info) => {
    const errors: string[] = [];
    page.on("pageerror", (e) => errors.push(e.message));
    await page.goto("/#/training");
    await page
      .getByRole("combobox", { name: "Training method", exact: true })
      .selectOption(method);
    await page
      .getByRole("button", { name: "Use smoke preset", exact: true })
      .click();
    await page
      .getByRole("textbox", { name: "Run name", exact: true })
      .fill(`M5 ${method}`);
    await page
      .getByRole("button", { name: "Start training", exact: true })
      .click();
    await expect(page).toHaveURL(/#\/training\//);
    const id = page.url().split("/training/")[1];
    const job = async (run = id): Promise<Job> =>
      (await page.request.get(`/api/training/runs/${run}`)).json();
    await expect
      .poll(async () => (await job()).state, { timeout: 60000 })
      .toBe("completed");
    await expect(page.locator(".job-state")).toHaveAttribute(
      "data-state",
      "completed",
    );
    const trained = await job();
    expect(trained.counters.games).toBe(method === "hybrid" ? 80 : 32);
    expect(trained.counters.truncated_games).toBe(0);
    expect(trained.counters.crossovers).toBe(0);
    expect(trained.population_progress?.selection_games).toBe(
      method === "hybrid" ? 64 : 32,
    );
    expect(trained.population_progress?.training_games).toBe(
      method === "hybrid" ? 16 : 0,
    );
    if (method === "hybrid")
      expect(trained.counters.updates).toBeGreaterThan(0);
    else expect(trained.counters.updates ?? 0).toBe(0);
    expect(JSON.stringify(trained)).not.toContain('"parameters"');
    await expect(page.locator(".watched-match")).toBeVisible();
    await page
      .getByRole("combobox", { name: "Candidate generation", exact: true })
      .selectOption("1");
    if (method === "hybrid")
      await expect(
        page.getByRole("heading", {
          name: "Participant replacements",
          exact: true,
        }),
      ).toBeVisible();
    const generation = await (
      await page.request.get(`/api/training/runs/${id}/generations/1`)
    ).json();
    expect(generation.neural_ranked).toHaveLength(method === "hybrid" ? 8 : 4);
    expect(JSON.stringify(generation)).not.toContain('"parameters"');
    await page
      .getByRole("combobox", { name: "Candidate", exact: true })
      .selectOption(generation.neural_ranked[1].id);
    await page
      .getByRole("textbox", { name: "Bot name", exact: true })
      .fill(`M5 ${method} bot`);
    await page
      .getByRole("button", { name: "Save candidate", exact: true })
      .click();
    await expect(
      page.getByRole("link", { name: "Evaluate this bot", exact: true }),
    ).toBeVisible();
    const bot = (
      (await (await page.request.get("/api/bots")).json()) as Bot[]
    ).find((b) => b.name === `M5 ${method} bot`)!;
    expect(bot.kind).toBe("tanh-56-32-1-v1");
    const modelPath = `${dataDir}/bots/${bot.id}/model.json`;
    const modelBefore = await readFile(modelPath, "utf8");
    const archive = JSON.parse(
      await readFile(`${dataDir}/runs/${id}/generations/0001.json`, "utf8"),
    );
    expect(JSON.parse(modelBefore).weights).toEqual(
      archive.ranked[1].parameters,
    );
    if (method === "hybrid") {
      expect(archive.replacements).toHaveLength(2);
      expect(archive.training_counters.games).toBe(8);
      expect(trained.counters.updates).toBeGreaterThan(0);
    }
    await page.screenshot({
      path: info.outputPath(`${method}-desktop.png`),
      fullPage: true,
    });
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({
      path: info.outputPath(`${method}-mobile.png`),
      fullPage: true,
    });
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    await page.setViewportSize({ width: 1280, height: 900 });
    await stop();
    await start();
    await page.reload();
    expect((await job()).counters).toEqual(trained.counters);
    await page.getByRole("link", { name: "My bots", exact: true }).click();
    const card = page.locator(".bot-card").filter({
      has: page.getByRole("heading", { name: bot.name, exact: true }),
    });
    await card
      .getByRole("link", { name: `Evaluate ${bot.name}`, exact: true })
      .click();
    await page
      .getByRole("spinbutton", { name: "Pairs per opponent", exact: true })
      .fill("1");
    await page
      .getByRole("button", { name: "Start evaluation", exact: true })
      .click();
    await expect(page).toHaveURL(/#\/evaluations\//);
    const evaluationID = page.url().split("/evaluations/")[1];
    await expect
      .poll(
        async () =>
          (
            await (
              await page.request.get(`/api/evaluations/${evaluationID}`)
            ).json()
          ).state,
        { timeout: 60000 },
      )
      .toBe("completed");
    const evaluation = await (
      await page.request.get(`/api/evaluations/${evaluationID}`)
    ).json();
    expect(evaluation.evaluation.stats.games).toBe(4);
    expect(evaluation.counters.updates ?? 0).toBe(0);
    // Keep this larger budget as a stop guard; do not finish a long training run.
    await page.goto("/#/training");
    await page
      .getByRole("combobox", { name: "Training method", exact: true })
      .selectOption(method);
    await page
      .getByRole("button", { name: "Use smoke preset", exact: true })
      .click();
    await page
      .getByRole("spinbutton", {
        name:
          method === "hybrid"
            ? "Training games per participant per round"
            : "Pairs per opponent",
        exact: true,
      })
      .fill("8");
    await page
      .getByRole("button", { name: "Start training", exact: true })
      .click();
    await expect(page).toHaveURL(/#\/training\//);
    const other = page.url().split("/training/")[1];
    await expect(page.locator(".watched-match")).toBeVisible();
    await page
      .getByRole("button", { name: "Pause playback", exact: true })
      .click();
    const viewer = page.locator(".watched-match");
    const frame = await viewer.getAttribute("data-frame");
    const before = await job(other);
    await expect
      .poll(async () => (await job(other)).counters.games)
      .toBeGreaterThan(before.counters.games);
    await expect(viewer).toHaveAttribute("data-frame", frame!);
    const player = await context.newPage();
    await player.clock.install();
    player.on("pageerror", (e) => errors.push(e.message));
    await player.goto("/#/bots");
    await player
      .getByRole("button", { name: `Play against ${bot.name}`, exact: true })
      .click();
    await expect(player).toHaveURL(/#\/games\//);
    const gameID = player.url().split("/games/")[1];
    await page
      .getByRole("button", { name: "Stop and checkpoint", exact: true })
      .click();
    await expect(page.locator(".job-state")).toHaveAttribute(
      "data-state",
      "stopped",
    );
    const stopped = await job(other);
    expect(stopped.counters.games).toBeLessThan(
      stopped.population_progress!.total_budget,
    );
    await stop();
    await start();
    await page.reload();
    await player.reload();
    expect((await job(other)).counters).toEqual(stopped.counters);
    await page.getByRole("button", { name: "Resume run", exact: true }).click();
    await expect
      .poll(async () => (await job(other)).counters.games)
      .toBeGreaterThan(stopped.counters.games);
    await page
      .getByRole("button", { name: "Stop and checkpoint", exact: true })
      .click();
    await expect(page.locator(".job-state")).toHaveAttribute(
      "data-state",
      "stopped",
    );
    let game = await snapshot(player, gameID);
    while (!game.continuations.complete) game = await firstStep(player, gameID);
    game = await buttonCommand(
      player,
      gameID,
      game.draft.length ? "Confirm turn" : "Pass turn",
    );
    expect(game.version).toBeGreaterThan(1);
    expect(await readFile(modelPath, "utf8")).toBe(modelBefore);
    await player.close();
    expect(errors).toEqual([]);
  });
}

test("research records every seed, plots measured budgets and preserves final confirmation after restart", async ({
  page,
}, info) => {
  const errors: string[] = [];
  page.on("pageerror", (e) => errors.push(e.message));
  await page.goto("/#/research");
  await expect(
    page.getByRole("heading", { name: "Compare the evidence." }),
  ).toBeVisible();
  const config = page.getByRole("textbox", {
    name: "Full experiment configuration",
  });
  await expect(config).toHaveValue(/"training_seeds"/);
  const defaults = JSON.parse(await config.inputValue());
  expect(defaults.training_seeds).toHaveLength(5);
  await page
    .getByRole("button", { name: "Use research smoke preset", exact: true })
    .click();
  const bounded = JSON.parse(await config.inputValue());
  expect(bounded.training_seeds).toEqual([11, 12]);
  // This real scenario verifies all five independent seeds at bounded per-seed
  // budgets; ordinary smoke remains the smaller two-seed lifecycle check.
  bounded.training_seeds = [11, 12, 13, 14, 15];
  await config.fill(JSON.stringify(bounded, null, 2));
  await page
    .getByRole("textbox", { name: "Experiment name", exact: true })
    .fill("Five-seed research");
  await page
    .getByRole("button", { name: "Start experiment", exact: true })
    .click();
  await expect(page).toHaveURL(/#\/research\//);
  const id = page.url().split("/research/")[1];
  const experiment = async () =>
    await (await page.request.get(`/api/experiments/${id}`)).json();
  await expect(page.getByTestId("research-state")).toHaveText("completed", {
    timeout: 90000,
  });
  const x = await experiment();
  expect(x.runs).toHaveLength(15);
  expect(x.methods).toHaveLength(3);
  expect(x.differences).toHaveLength(3);
  expect(x.counters.games).toBe(319);
  expect(x.selection_locked).toBe(true);
  expect(x.verdict.status).toBe("candidate");
  await expect(page.getByTestId("research-verdict")).toContainText(
    "Candidate retained",
  );
  await expect(page.getByRole("img")).toHaveCount(2);
  for (const run of x.runs) {
    const dots = page.locator(
      `.research-chart g[data-seed="${run.seed}"][data-algorithm="${run.algorithm}"]`,
    );
    await expect(dots).toHaveCount(2);
    expect(Number(await dots.nth(0).getAttribute("data-budget"))).toBe(
      run.counters.decisions,
    );
    expect(Number(await dots.nth(1).getAttribute("data-budget"))).toBe(
      run.wall_seconds,
    );
  }
  const reportResponse = await page.request.get(
    `/api/experiments/${id}/report`,
  );
  expect(reportResponse.ok()).toBe(true);
  const reportBytes = await reportResponse.body();
  const { createHash } = await import("node:crypto");
  expect(createHash("sha256").update(reportBytes).digest("hex")).toBe(
    x.report_sha256,
  );
  const report = JSON.parse(reportBytes.toString());
  expect(report.runs).toEqual(x.runs);
  expect(report.final_scores).toEqual(x.final_scores);
  await page.screenshot({
    path: info.outputPath("research-desktop.png"),
    fullPage: true,
  });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.screenshot({
    path: info.outputPath("research-mobile.png"),
    fullPage: true,
  });
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await stop();
  await start();
  await page.reload();
  await expect(page.getByTestId("research-state")).toHaveText("completed");
  const after = await experiment();
  expect(after.candidate_id).toBe(x.candidate_id);
  expect(after.counters).toEqual(x.counters);
  expect(after.report_sha256).toBe(x.report_sha256);
  expect(after.verdict).toEqual(x.verdict);
  const repeat = await page.request.post(`/api/experiments/${id}/resume`, {
    data: { command_id: "repeat-final", expected_version: after.version },
  });
  expect(repeat.status()).toBe(409);
  await page
    .getByRole("link", { name: "Evaluate saved candidate", exact: true })
    .click();
  await expect(page).toHaveURL(/#\/evaluations\?bot=/);
  await page.getByRole("link", { name: "Research", exact: true }).click();
  await page
    .getByRole("button", { name: "Use research smoke preset", exact: true })
    .click();
  const nextConfig = JSON.parse(await config.inputValue());
  nextConfig.methods = nextConfig.methods.filter(
    (m: { algorithm: string }) => m.algorithm === "td-lambda",
  );
  nextConfig.methods[0].td.games = 32;
  nextConfig.methods[0].td.max_turns = 50;
  await config.fill(JSON.stringify(nextConfig, null, 2));
  await page
    .getByRole("textbox", { name: "Experiment name", exact: true })
    .fill("Research resume and final-data reuse");
  await page
    .getByRole("button", { name: "Start experiment", exact: true })
    .click();
  await expect(page).toHaveURL(/#\/research\//);
  const resumedID = page.url().split("/research/")[1];
  const resumeSnapshot = async () =>
    await (await page.request.get(`/api/experiments/${resumedID}`)).json();
  await expect
    .poll(async () => (await resumeSnapshot()).counters.games)
    .toBeGreaterThan(0);
  await page
    .getByRole("button", { name: "Stop experiment", exact: true })
    .click();
  await expect(page.getByTestId("research-state")).toHaveText("stopped");
  const stopped = await resumeSnapshot();
  expect(stopped.can_resume).toBe(true);
  await stop();
  await start();
  await page.reload();
  await expect(page.getByTestId("research-state")).toHaveText("stopped");
  await page
    .getByRole("button", { name: "Resume experiment", exact: true })
    .click();
  await expect(page.getByTestId("research-state")).toHaveText("completed", {
    timeout: 90000,
  });
  const resumed = await resumeSnapshot();
  expect(resumed.counters.games).toBe(72);
  expect(resumed.counters.decisions).toBeGreaterThan(
    stopped.counters.decisions,
  );
  expect(resumed.final_data_role).toBe("reused-development");
  expect(resumed.verdict.status).toBe("candidate");
  await expect(page.getByTestId("research-verdict")).toContainText(
    "previously reserved",
  );
  expect(errors).toEqual([]);
});

test("play first: remember opponent and color, keep drafts across tabs and resume from home", async ({
  page,
}) => {
  await page.goto("/#/play/new");
  await expect(
    page.getByRole("heading", { name: "Heuristic", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Change", exact: true }).click();
  await page
    .getByLabel("Choose opponent", { exact: true })
    .selectOption("builtin/random-v1");
  await page.reload();
  await expect(
    page.getByRole("heading", { name: "Random", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("navigation", { name: "Main navigation" })
    .getByRole("link", { name: "Settings" })
    .click();
  await page.getByLabel("Your color", { exact: true }).selectOption("1");
  await page.getByRole("link", { name: "Play", exact: true }).click();
  await expect(page.getByText("You play Black", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Start game", exact: true }).click();
  await expect(page).toHaveURL(/#\/games\//);
  const id = page.url().split("/games/")[1];
  let game = await snapshot(page, id);
  expect(game.bot_id).toBe("builtin/random-v1");
  expect(game.human).toBe(1);
  await rendered(page, game);
  game = await firstStep(page, id);
  await page.getByRole("link", { name: "Training", exact: true }).click();
  await expect(
    page.getByRole("navigation", { name: "Training navigation" }),
  ).toBeVisible();
  await page.getByRole("link", { name: "Play", exact: true }).click();
  await rendered(page, game);
  expect(await snapshot(page, id)).toEqual(game);
  await page.goto("/");
  await expect(
    page.getByRole("link", { name: "Continue game", exact: true }),
  ).toHaveAttribute("href", `#/games/${id}`);
  for (let point = 0; point < 24; point++) {
    const position = game.draft_position;
    const count = position.checkers[0][point] + position.checkers[1][point];
    const player = position.checkers[0][point] > 0 ? "White" : "Black";
    await expect(page.locator(`[data-point="${point}"]`)).toHaveAttribute(
      "aria-label",
      `Point ${point + 1}, ${count ? `${count} ${player} checkers` : "empty"}`,
    );
  }
  await page.getByRole("link", { name: "Continue game", exact: true }).click();
  await rendered(page, game);
  expect(await snapshot(page, id)).toEqual(game);
});

test("default play works without history or evaluations and falls back from an unavailable bot", async ({
  page,
}) => {
  await page.addInitScript(() =>
    localStorage.setItem("evonardy.bot", "deleted-model"),
  );
  await page.route("**/api/games", async (route) => {
    if (route.request().method() === "GET")
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: { message: "history unavailable" } }),
      });
    else await route.continue();
  });
  await page.route("**/api/evaluations", (route) => route.abort());
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "Heuristic", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByText("Your previous opponent is unavailable.", { exact: false }),
  ).toBeVisible();
  await expect(page.getByRole("alert")).toContainText(
    "Could not load saved games",
  );
  await page.getByRole("button", { name: "Start game", exact: true }).click();
  await expect(page).toHaveURL(/#\/games\//);
  const game = await snapshot(page, page.url().split("/games/")[1]);
  expect(game.bot_id).toBe("builtin/heuristic-v1");
  expect(game.human).toBe(0);
});

test("blocked browser storage does not prevent playing", async ({ page }) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, "localStorage", {
      get() {
        throw new Error("storage disabled");
      },
    });
  });
  await page.goto("/#/play/new");
  await page.getByRole("button", { name: "Change", exact: true }).click();
  await page
    .getByLabel("Choose opponent", { exact: true })
    .selectOption("builtin/random-v1");
  await page.getByRole("button", { name: "Start game", exact: true }).click();
  await expect(page).toHaveURL(/#\/games\//);
  const game = await snapshot(page, page.url().split("/games/")[1]);
  expect(game.bot_id).toBe("builtin/random-v1");
});

test("failed preference writes keep the current session color instead of an old stored value", async ({
  page,
}) => {
  await page.addInitScript(() => {
    localStorage.setItem("evonardy.color", "0");
    Storage.prototype.setItem = () => {
      throw new Error("quota exceeded");
    };
  });
  await page.goto("/#/settings");
  await page.getByLabel("Your color", { exact: true }).selectOption("1");
  await page.getByRole("link", { name: "Play", exact: true }).click();
  // Other scenarios may have left a saved game in the shared server archive.
  const newGame = page.getByRole("link", { name: "New game", exact: true });
  const start = page.getByRole("button", { name: "Start game", exact: true });
  await expect(newGame.or(start)).toBeVisible();
  if (await newGame.isVisible()) await newGame.click();
  await expect(page.getByText("You play Black", { exact: true })).toBeVisible();
  await start.click();
  await expect(page).toHaveURL(/#\/games\//);
  const game = await snapshot(page, page.url().split("/games/")[1]);
  expect(game.human).toBe(1);
});
