import { test, expect, type Page } from "@playwright/test";
import { spawn, type ChildProcess } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { fileURLToPath } from "node:url";
import type { Bot, Snapshot } from "../src/api";
import type { Job } from "../src/jobs";

const root = fileURLToPath(new URL("../../", import.meta.url));
const baseURL = "http://127.0.0.1:18180";
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

async function snapshot(page: Page, id: string): Promise<Snapshot> {
  const response = await page.request.get(`/api/games/${id}`);
  expect(response.ok()).toBeTruthy();
  return response.json();
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
    .poll(async () => (await snapshot(page, id)).version)
    .toBe(before.version + 1);
  const x = await snapshot(page, id);
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
    .poll(async () => (await snapshot(page, id)).version)
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
  await page.goto("/");
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
  if (x.phase === "awaiting_roll")
    x = await buttonCommand(page, id, "Roll dice");
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
    if (x.phase === "awaiting_roll")
      x = await buttonCommand(page, id, "Roll dice");
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
  await page.getByRole("link", { name: "← My bots", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Opening study", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".recent-game")).toContainText("wins");
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
  await page.goto("/");
  const saved = page.locator(".bot-card").filter({
    has: page.getByRole("heading", { name: bot.name, exact: true }),
  });
  await expect(saved).toBeVisible();
  await page.getByRole("combobox", { name: "Your color" }).selectOption("1");
  await saved
    .getByRole("button", { name: `Play against ${bot.name}`, exact: true })
    .click();
  await expect(page).toHaveURL(/#\/games\//);
  const id = page.url().split("/games/")[1];
  let x = await snapshot(page, id);
  await rendered(page, x);
  expect(x.human).toBe(1);
  if (x.phase === "awaiting_roll")
    x = await buttonCommand(page, id, "Roll dice");
  const observer = await context.newPage();
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
  await player.goto("/");
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
    if (x.phase === "awaiting_roll")
      x = await buttonCommand(player, gameID, "Roll dice");
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
  await page.goto("/");
  const card = page
    .locator(".bot-card")
    .filter({
      has: page.getByRole("heading", { name: bot.name, exact: true }),
    });
  await expect(card).toContainText("4 games");
  expect(errors).toEqual([]);
});
