import { defineConfig } from "@playwright/test";
export default defineConfig({
  testDir: "./e2e",
  workers: 1,
  fullyParallel: false,
  timeout: 240_000,
  use: {
    baseURL: "http://127.0.0.1:18180",
    viewport: { width: 1280, height: 900 },
    trace: "retain-on-failure",
    screenshot: process.env.CI ? "only-on-failure" : "off",
  },
  reporter: process.env.CI ? [["list"], ["html", { open: "never" }]] : "list",
});
