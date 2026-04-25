import { defineConfig, devices } from '@playwright/test'

const port = Number(process.env.TLD_E2E_PORT ?? 8060)
const baseURL = process.env.TLD_E2E_BASE_URL ?? `http://127.0.0.1:${port}`
const dataDir = process.env.TLD_E2E_DATA_DIR ?? `/tmp/tld-playwright-${process.env.GITHUB_RUN_ID ?? 'local'}`
const binary = process.env.TLD_E2E_BINARY ?? 'tld'

export default defineConfig({
  testDir: './e2e',
  timeout: 45_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  retries: process.env.CI ? 2 : 0,
  workers: 1,
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    viewport: { width: 1440, height: 1000 },
    actionTimeout: 10_000,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
  webServer: {
    command: `${binary} serve --foreground --host 127.0.0.1 --port ${port} --data-dir ${dataDir}`,
    url: `${baseURL}/api/ready`,
    timeout: 30_000,
    reuseExistingServer: !process.env.CI,
    cwd: '..',
  },
})
