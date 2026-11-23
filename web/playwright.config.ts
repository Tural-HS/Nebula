import { defineConfig, devices } from "@playwright/test";

/*
 * End-to-end tests against the real stack: Postgres, Keycloak, Mailpit and the
 * API from `make up`, plus the production web build served by `vite preview`.
 * Every spec creates its own workspace, so runs never touch other data, and
 * the teardown deletes this run's workspaces again.
 */
const CI = Boolean(process.env.CI);

export default defineConfig({
  testDir: "./e2e",
  globalSetup: "./e2e/global-setup.ts",
  globalTeardown: "./e2e/global-teardown.ts", // set E2E_KEEP_DATA=1 to inspect the data afterwards
  timeout: 60_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: CI ? 1 : 2,
  retries: CI ? 1 : 0,
  forbidOnly: CI,
  reporter: CI ? [["github"], ["html", { open: "never" }]] : [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: "http://localhost:5173",
    locale: "az-AZ",
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    { name: "setup", testMatch: /auth\.setup\.ts/ },
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"], viewport: { width: 1280, height: 860 } },
      dependencies: ["setup"],
      testIgnore: /auth\.setup\.ts/,
    },
  ],
  webServer: {
    command: "npm run build && npx vite preview",
    url: "http://localhost:5173",
    reuseExistingServer: !CI, // locally, an already running `make web-dev` is fine
    timeout: 120_000,
  },
});
