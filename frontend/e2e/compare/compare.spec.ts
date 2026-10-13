import { readFileSync } from 'node:fs'
import { test, expect } from '../fixtures'

const hash = '#' + readFileSync(new URL('../../../internal/mermaid/fixtures/scene-url.txt', import.meta.url), 'utf8').trim().split('#')[1]

test('self-contained comparison renders with API requests blocked', async ({ page }) => {
  const calls: string[] = []
  await page.route('**/api/**', (route) => { calls.push(route.request().url()); return route.abort() })
  await page.goto('/compare' + hash)
  await expect(page.getByTestId('comparison-scene-page')).toBeVisible()
  await expect(page.getByTestId('repository-change-canvas').locator('canvas')).toBeVisible()
  await expect(page.getByTestId('repository-change-mermaid').locator('svg')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Open in TLD' })).toHaveAttribute('href', 'tld://compare' + hash)
  await expect(page.getByTestId('repositories-view-mode')).toHaveCount(0)
  expect(calls).toEqual([])
  await page.goto('/compare#v2=bad')
  await expect(page.getByRole('alert')).toContainText('Unsupported comparison URL')
})

test('desktop startup waits for queued navigation before mounting the workspace', async ({ page }) => {
  const calls: string[] = []
  await page.route('**/api/**', (route) => { calls.push(route.request().url()); return route.abort() })
  await page.addInitScript((hash) => {
    window.__TLD_APP__ = true
    window.__TLD_PLATFORM__ = 'windows'
    let pending = [{ route: '/compare' + hash }]
    // Model the native bridge while using a real browser/router transition.
    Object.assign(window, {
      go: { main: { DesktopBridge: { DrainSceneLinks: async () => { const result = pending; pending = []; return result } } } },
      runtime: {
        EventsOn: () => () => {},
        WindowIsMaximised: async () => false,
        WindowMinimise: () => {},
        WindowToggleMaximise: () => {},
        Quit: () => {},
      },
    })
  }, hash)
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'Close window', exact: true })).toBeVisible()
  await expect(page.getByTestId('repository-change-mermaid').locator('svg')).toBeVisible()
  await expect(page.getByRole('link', { name: 'Open in TLD' })).toHaveCount(0)
  expect(calls).toEqual([])
})
