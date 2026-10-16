import { readFileSync } from 'node:fs'
import { test, expect } from '../fixtures'
import { waitForStableZuiCamera, zuiViewState } from '../helpers/zui'

const hash = '#' + readFileSync(new URL('../../../internal/mermaid/fixtures/scene-url.txt', import.meta.url), 'utf8').trim().split('#')[1]

test('diagram zoom uses native SVG sizing without moving the repository canvas', async ({ page }) => {
  await page.route((url) => url.pathname.startsWith('/api/'), (route) => route.abort())
  await page.goto('/compare?debugZuiTest=1' + hash)
  const diagram = page.getByTestId('repository-change-mermaid').locator('.tld-mermaid-preview__body svg')
  await expect(diagram).toBeVisible()
  await expect(page.locator('.tld-mermaid-preview__header').getByRole('button', { name: 'Zoom in diagram', exact: true })).toBeVisible()
  const header = (await page.locator('.tld-mermaid-preview__header').boundingBox())!
  const reset = (await page.getByRole('button', { name: 'Reset diagram zoom', exact: true }).boundingBox())!
  expect(Math.abs(reset.x + reset.width / 2 - header.x - header.width / 2)).toBeLessThan(1)
  expect(await page.locator('.tld-mermaid-preview__body').evaluate((element) => ({
    padding: getComputedStyle(element).padding,
    overflow: getComputedStyle(element).overflow,
  }))).toEqual({ padding: '0px', overflow: 'visible' })
  expect(await page.getByTestId('markdown-preview').evaluate((element) => getComputedStyle(element).padding)).toBe('0px')
  const originalWidth = (await diagram.boundingBox())!.width
  const camera = await waitForStableZuiCamera(page)
  await page.getByRole('button', { name: 'Zoom in diagram', exact: true }).click()
  await expect.poll(async () => (await diagram.boundingBox())!.width).toBeGreaterThan(originalWidth * 1.2)
  expect(await zuiViewState(page)).toEqual(camera)
  const viewport = page.getByTestId('repository-change-mermaid')
  await viewport.evaluate((element) => { element.scrollLeft = 40 })
  expect(await viewport.evaluate((element) => element.scrollLeft)).toBeGreaterThan(0)
  await page.getByRole('button', { name: 'Reset diagram zoom', exact: true }).click()
  await expect.poll(async () => Math.round((await diagram.boundingBox())!.width)).toBe(Math.round(originalWidth))
})

for (const width of [380, 1000]) {
  test(`change diagram scrolls without interacting with the canvas at ${width}px`, async ({ page, browserName, isMobile }) => {
    await page.addInitScript((width) => {
      localStorage.setItem('tld:repositories:mermaidPaneWidth', String(width))
    }, width)
    await page.route((url) => url.pathname.startsWith('/api/'), (route) => route.abort())
    await page.goto('/compare?debugZuiTest=1' + hash)
    const body = page.getByTestId('repository-change-mermaid')
    await expect(body.locator('svg')).toBeVisible()
    // Model a long comparison without relying on a particular Mermaid layout.
    await body.locator('[data-testid="markdown-preview"]').evaluate((preview) => {
      for (let i = 0; i < 100; i++) {
        const paragraph = document.createElement('p')
        paragraph.textContent = `Changed file ${i}`
        preview.append(paragraph)
      }
    })
    const before = await waitForStableZuiCamera(page)
    await body.hover()
    if (browserName === 'webkit' && isMobile) {
      // Playwright cannot send native wheel input to mobile WebKit. Check that
      // scrolling is allowed, then verify the responsive panel can scroll.
      const allowed = await body.evaluate((element) => {
        const rect = element.getBoundingClientRect()
        return element.dispatchEvent(new WheelEvent('wheel', {
          bubbles: true, cancelable: true, deltaY: 400,
          clientX: rect.left + rect.width / 2, clientY: rect.top + rect.height / 2,
        }))
      })
      expect(allowed).toBe(true)
      await body.evaluate((element) => element.scrollBy(0, 400))
    } else {
      await page.mouse.wheel(0, 400)
    }
    await expect.poll(() => body.evaluate((element) => element.scrollTop)).toBeGreaterThan(0)
    expect(await zuiViewState(page)).toEqual(before)
    await expect(page.getByTestId('zui-hover-popover')).toBeHidden()

    // Canvas hover detection listens on window; panel movement must stop there.
    await page.evaluate(() => {
      document.documentElement.dataset.panelMouseMoves = '0'
      window.addEventListener('mousemove', () => {
        const root = document.documentElement
        root.dataset.panelMouseMoves = String(Number(root.dataset.panelMouseMoves) + 1)
      })
    })
    await body.dispatchEvent('mousemove', { clientX: 800, clientY: 400 })
    await expect(page.locator('html')).toHaveAttribute('data-panel-mouse-moves', '0')
  })
}

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
