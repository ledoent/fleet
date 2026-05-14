import { defineConfig } from "@playwright/test";

const BASE_URL = process.env.FLEET_URL ?? "https://fleet.hz.ledoweb.com";

export default defineConfig({
  testDir: "./tests",
  fullyParallel: true,
  workers: 8,
  reporter: [["list"], ["json", { outputFile: "results.json" }]],
  use: {
    baseURL: BASE_URL,
    ignoreHTTPSErrors: true,
    extraHTTPHeaders: {
      Authorization: `Bearer ${requireToken()}`,
    },
  },
  expect: { timeout: 30_000 },
  timeout: 60_000,
});

function requireToken(): string {
  const t = process.env.FLEET_TOKEN;
  if (!t) {
    throw new Error(
      "FLEET_TOKEN env var is required. Run: export FLEET_TOKEN=$(awk '/token:/ {print $2}' ~/.fleet/config)",
    );
  }
  return t;
}
