import { defineConfig, devices } from '@playwright/test'

function resolveWorkers() {
  const override = process.env.TLD_E2E_WORKERS?.trim()
  if (override) {
    const numeric = Number(override)
    return Number.isFinite(numeric) ? numeric : override
  }
  return process.env.CI ? 2 : undefined
}

export default defineConfig({
  testDir: './e2e',
  timeout: 45_000,
  expect: { timeout: 10_000 },
  fullyParallel: true,
  retries: process.env.CI ? 2 : 0,
  workers: resolveWorkers(),
  reporter: process.env.CI ? [['github'], ['html', { open: 'never' }]] : [['list'], ['html', { open: 'never' }]],
  use: {
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
    {
      name: 'firefox',
      use: { ...devices['Desktop Firefox'] },
    },
    {
      name: 'webkit',
      use: { ...devices['Desktop Safari'] },
    },
    {
      name: 'mobile-safari',
      use: { ...devices['iPhone 14'] },
    },
    {
      name: 'mobile-chrome',
      use: { ...devices['Pixel 7'] },
    },
    {
      name: 'tablet-touch',
      use: { ...devices['iPad Pro 11'] },
    },
    {
      name: 'desktop-touch-chromium',
      use: {
        ...devices['Desktop Chrome'],
        hasTouch: true,
        viewport: { width: 1366, height: 900 },
      },
    },
  ],
})
