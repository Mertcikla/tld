import { expect, test } from '../fixtures'
import {
  dispatchTouchEventOnLocator,
  dispatchWheelOnLocator,
} from '../helpers/vieweditor'
import {
  canvasBox,
  createNestedZuiFixture,
  createTaggedZuiFixture,
  dispatchZuiDoubleClick,
  dispatchZuiMousePan,
  dispatchZuiTouchPan,
  dispatchZuiWheel,
  waitForStableZuiCamera,
  waitForZuiFrame,
  waitForZuiReady,
  zuiLastInteraction,
  zuiScreenGeometry,
  zuiViewState,
} from '../helpers/zui'

function stateDelta(
  left: { x: number; y: number; originX?: number; originY?: number; zoom?: number },
  right: { x: number; y: number; originX?: number; originY?: number; zoom?: number },
) {
  const leftOriginX = typeof left.originX === 'number' ? left.originX : 0
  const leftOriginY = typeof left.originY === 'number' ? left.originY : 0
  const rightOriginX = typeof right.originX === 'number' ? right.originX : 0
  const rightOriginY = typeof right.originY === 'number' ? right.originY : 0
  const zoom = Math.max(
    typeof left.zoom === 'number' ? left.zoom : 1,
    typeof right.zoom === 'number' ? right.zoom : 1,
  )
  return (
    Math.abs(left.x - right.x) +
    Math.abs(left.y - right.y) +
    Math.abs(leftOriginX - rightOriginX) * zoom +
    Math.abs(leftOriginY - rightOriginY) * zoom
  )
}

test('zooms with physical wheel and ctrl-wheel pinch while trackpad-like wheels pan', async ({ page }) => {
  const { root } = await createNestedZuiFixture(page)
  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await waitForZuiReady(page)

  const box = await canvasBox(page)
  const center = { x: box.x + box.width / 2, y: box.y + box.height / 2 }
  await waitForStableZuiCamera(page)
  await dispatchZuiWheel(page, center, { deltaY: 160, deltaMode: 0 })
  await expect.poll(async () => (await zuiLastInteraction(page))?.mode).toBe('wheel-zoom')

  await waitForStableZuiCamera(page)
  await dispatchZuiWheel(page, center, { deltaY: -140, deltaMode: 0, ctrlKey: true })
  await expect.poll(async () => (await zuiLastInteraction(page))?.mode).toBe('ctrl-wheel-pinch')

  const afterCtrlWheel = await waitForStableZuiCamera(page)
  const isMobileLayout = await page.evaluate(() => window.matchMedia('(max-width: 767px)').matches)
  await dispatchZuiWheel(page, center, { deltaX: 80, deltaY: 28, deltaMode: 0 })
  if (isMobileLayout) {
    await waitForZuiFrame(page)
    expect(stateDelta(await zuiViewState(page), afterCtrlWheel)).toBeLessThan(1)
  } else {
    await expect.poll(async () => (await zuiLastInteraction(page))?.mode).toBe('trackpad-pan')
  }
  const afterTrackpad = await waitForStableZuiCamera(page)
  expect(Math.abs(afterTrackpad.zoom - afterCtrlWheel.zoom)).toBeLessThan(0.03)
})

test('pans with mouse drag and zooms with double-click', async ({ page }) => {
  const { root } = await createNestedZuiFixture(page)
  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await waitForZuiReady(page)

  const box = await canvasBox(page)
  const beforePan = await zuiViewState(page)
  await dispatchZuiMousePan(
    page,
    { x: box.x + box.width * 0.52, y: box.y + box.height * 0.54 },
    { x: box.x + box.width * 0.52 + 150, y: box.y + box.height * 0.54 + 90 },
  )
  await expect.poll(async () => stateDelta(await zuiViewState(page), beforePan)).toBeGreaterThan(30)

  const beforeDoubleClick = await zuiViewState(page)
  await dispatchZuiDoubleClick(page, { x: box.x + box.width * 0.48, y: box.y + box.height * 0.48 })
  const isMobileLayout = await page.evaluate(() => window.matchMedia('(max-width: 767px)').matches)
  if (isMobileLayout) {
    await expect.poll(async () => (await zuiLastInteraction(page))?.mode).toBe('double-click-zoom')
  } else {
    await expect.poll(async () => (await zuiViewState(page)).zoom).toBeGreaterThan(beforeDoubleClick.zoom + 0.05)
  }
})

test('keeps hover popovers interactive and breadcrumb actions update the camera', async ({ page }) => {
  const { root, parent } = await createNestedZuiFixture(page)
  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await waitForZuiReady(page)
  await waitForStableZuiCamera(page)

  const { node } = await zuiScreenGeometry(page, { nodeElementId: parent.id }) as {
    node: { x: number; y: number; width: number; height: number }
  }
  await page.evaluate((point) => {
    window.dispatchEvent(new MouseEvent('mousemove', {
      bubbles: true,
      cancelable: true,
      clientX: point.x,
      clientY: point.y,
    }))
  }, { x: node.x + node.width / 2, y: node.y + node.height / 2 })
  await expect(page.getByTestId('zui-hover-popover')).toBeVisible()
  await expect(page.getByTestId('zui-hover-popover')).toContainText(parent.name)
  await page.getByTestId('zui-hover-popover').hover()
  const openAction = page.getByRole('link', { name: /Open (in Editor|Diagram)/ })
  await expect(openAction).toBeVisible()
  await openAction.click()
  await expect(page).toHaveURL(/\/views\/\d+/)

  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}&element=${parent.id}`)
  await waitForZuiReady(page)
  await expect(page.getByTestId('zui-breadcrumb')).toBeVisible()
  const beforeBreadcrumb = await waitForStableZuiCamera(page)
  await page.getByTestId('zui-breadcrumb').getByText(root.name).click()
  await expect.poll(async () => stateDelta(await zuiViewState(page), beforeBreadcrumb)).toBeGreaterThan(8)
})

test('handles one-finger touch pan, two-finger pinch, and transition back to touch pan', async ({ page }) => {
  const { root } = await createNestedZuiFixture(page)
  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await waitForZuiReady(page)
  const box = await canvasBox(page)
  const center = { x: box.x + box.width / 2, y: box.y + box.height / 2 }

  const beforePan = await zuiViewState(page)
  await dispatchZuiTouchPan(page, { x: center.x - 70, y: center.y - 20 }, { x: center.x + 30, y: center.y + 60 })
  await expect.poll(async () => stateDelta(await zuiViewState(page), beforePan)).toBeGreaterThan(20)

  const beforePinch = await zuiViewState(page)
  const canvas = page.locator('canvas')
  await dispatchTouchEventOnLocator(canvas, 'touchstart', [
    { identifier: 1, clientX: center.x - 48, clientY: center.y },
    { identifier: 2, clientX: center.x + 48, clientY: center.y },
  ])
  await dispatchTouchEventOnLocator(canvas, 'touchmove', [
    { identifier: 1, clientX: center.x - 92, clientY: center.y - 14 },
    { identifier: 2, clientX: center.x + 92, clientY: center.y - 14 },
  ])
  await expect.poll(async () => (await zuiLastInteraction(page))?.mode).toBe('two-finger-pinch')
  await dispatchTouchEventOnLocator(canvas, 'touchend', [
    { identifier: 1, clientX: center.x - 92, clientY: center.y - 14 },
  ], [
    { identifier: 2, clientX: center.x + 92, clientY: center.y - 14 },
  ])
  await dispatchTouchEventOnLocator(canvas, 'touchmove', [
    { identifier: 1, clientX: center.x - 42, clientY: center.y + 38 },
  ])
  await dispatchTouchEventOnLocator(canvas, 'touchend', [], [
    { identifier: 1, clientX: center.x - 42, clientY: center.y + 38 },
  ])
  const isMobileLayout = await page.evaluate(() => window.matchMedia('(max-width: 767px)').matches)
  if (isMobileLayout) {
    await expect.poll(async () => (await zuiLastInteraction(page))?.mode).toBe('one-finger-pan')
  } else {
    await expect.poll(async () => (await zuiViewState(page)).zoom).toBeGreaterThan(beforePinch.zoom + 0.03)
  }
  await expect.poll(async () => stateDelta(await zuiViewState(page), beforePinch)).toBeGreaterThan(25)
})

test('lets native-wheel overlays scroll without moving the ZUI camera', async ({ page }) => {
  const { diagram, tagName, layerName } = await createTaggedZuiFixture(page)
  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${diagram.id}`)
  await waitForZuiReady(page)
  await page.getByTestId('zui-tags-button').click()
  await expect(page.getByTestId('zui-tags-panel')).toBeVisible()
  await expect(page.getByTestId('zui-tags-panel')).toContainText(tagName)
  await expect(page.getByTestId('zui-tags-panel')).toContainText(layerName)
  const before = await waitForStableZuiCamera(page)
  await dispatchWheelOnLocator(page.getByTestId('zui-tags-panel'), { deltaY: 220, deltaMode: 0 })
  const after = await waitForStableZuiCamera(page)
  expect(Math.abs(after.zoom - before.zoom)).toBeLessThan(0.02)
  expect(stateDelta(after, before)).toBeLessThan(4)
})
