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
  dispatchZuiSafariGesture,
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

test('pans with left and middle mouse drags, ignores right drag, and zooms with double-click', async ({ page }, testInfo) => {
  const { root } = await createNestedZuiFixture(page)
  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await waitForZuiReady(page)

  const box = await canvasBox(page)
  await dispatchZuiMousePan(
    page,
    { x: box.x + box.width * 0.52, y: box.y + box.height * 0.54 },
    { x: box.x + box.width * 0.52 + 150, y: box.y + box.height * 0.54 + 90 },
  )
  await expect.poll(async () => (await zuiLastInteraction(page))?.button).toBe(0)
  await expect.poll(async () => {
    const interaction = await zuiLastInteraction(page)
    return Math.abs(interaction?.deltaX ?? 0) + Math.abs(interaction?.deltaY ?? 0)
  }).toBeGreaterThan(200)

  const isMobileDeviceProject = ['mobile-safari', 'mobile-chrome', 'tablet-touch'].includes(testInfo.project.name)
  if (!isMobileDeviceProject) {
    const beforeMiddlePan = await waitForStableZuiCamera(page)
    await dispatchZuiMousePan(
      page,
      { x: box.x + box.width * 0.44, y: box.y + box.height * 0.56 },
      { x: box.x + box.width * 0.44 + 120, y: box.y + box.height * 0.56 + 80 },
      { button: 'middle' },
    )
    await expect.poll(async () => (await zuiLastInteraction(page))?.button).toBe(1)
    await expect.poll(async () => {
      const interaction = await zuiLastInteraction(page)
      return Math.abs(interaction?.deltaX ?? 0) + Math.abs(interaction?.deltaY ?? 0)
    }).toBeGreaterThan(150)
    expect(stateDelta(await zuiViewState(page), beforeMiddlePan)).toBeGreaterThan(0)

    const beforeRightDrag = await waitForStableZuiCamera(page)
    const beforeRightInteraction = await zuiLastInteraction(page)
    await dispatchZuiMousePan(
      page,
      { x: box.x + box.width * 0.40, y: box.y + box.height * 0.58 },
      { x: box.x + box.width * 0.40 + 120, y: box.y + box.height * 0.58 + 80 },
      { button: 'right' },
    )
    await waitForZuiFrame(page)
    expect(stateDelta(await zuiViewState(page), beforeRightDrag)).toBeLessThan(4)
    expect((await zuiLastInteraction(page))?.button).toBe(beforeRightInteraction?.button)
  }

  await dispatchZuiWheel(page, { x: box.x + box.width * 0.5, y: box.y + box.height * 0.5 }, { deltaY: 180, deltaMode: 0 })
  await expect.poll(async () => (await zuiLastInteraction(page))?.mode).toBe('wheel-zoom')
  await waitForStableZuiCamera(page)
  await dispatchZuiDoubleClick(page, { x: box.x + box.width * 0.48, y: box.y + box.height * 0.48 })
  await expect.poll(async () => (await zuiLastInteraction(page))?.mode).toBe('double-click-zoom')
})

test('zooms with Safari gesture events around the ZUI canvas point', async ({ page }) => {
  const { root } = await createNestedZuiFixture(page)
  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await waitForZuiReady(page)

  const box = await canvasBox(page)
  const center = { x: box.x + box.width * 0.54, y: box.y + box.height * 0.48 }
  await waitForStableZuiCamera(page)
  await dispatchZuiSafariGesture(page, center, 1, 0.7)
  await expect.poll(async () => (await zuiLastInteraction(page))?.mode).toBe('safari-gesture-pinch')
  await expect.poll(async () => (await zuiLastInteraction(page))?.factor ?? 1).toBeLessThan(0.8)
  await expect.poll(async () => {
    const interaction = await zuiLastInteraction(page)
    if (typeof interaction?.zoomBefore !== 'number' || typeof interaction.zoomAfter !== 'number') return 0
    return Math.abs(interaction.zoomAfter - interaction.zoomBefore)
  }).toBeGreaterThan(0.01)
})

test('keeps hover popovers interactive and breadcrumb actions update the camera', async ({ page }, testInfo) => {
  const { root, parent } = await createNestedZuiFixture(page)
  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await waitForZuiReady(page)
  await waitForStableZuiCamera(page)

  const { node } = await zuiScreenGeometry(page, { nodeElementId: parent.id }) as {
    node: { x: number; y: number; width: number; height: number }
  }
  const isMobileDeviceProject = ['mobile-safari', 'mobile-chrome', 'tablet-touch'].includes(testInfo.project.name)
  if (isMobileDeviceProject) {
    await page.mouse.move(node.x + node.width / 2, node.y + node.height / 2)
  } else {
    await page.mouse.move(node.x + node.width / 2, node.y + node.height / 2)
    await expect(page.getByTestId('zui-hover-popover')).toBeVisible()
    await expect(page.getByTestId('zui-hover-popover')).toContainText(parent.name)
    await page.getByTestId('zui-hover-popover').hover()
    const openAction = page.getByRole('link', { name: /Open (in Editor|Diagram)/ })
    await expect(openAction).toBeVisible()
    await openAction.click()
    await expect(page).toHaveURL(/\/views\/\d+/)
  }

  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}&element=${parent.id}`)
  await waitForZuiReady(page)
  await expect(page.getByTestId('zui-breadcrumb')).toBeVisible()
  const beforeBreadcrumb = await waitForStableZuiCamera(page)
  await page.getByTestId('zui-breadcrumb').getByText(root.name).click()
  await expect.poll(async () => stateDelta(await zuiViewState(page), beforeBreadcrumb)).toBeGreaterThan(8)
})

test('handles one-finger touch pan, two-finger pinch, and transition back to touch pan', async ({ page }, testInfo) => {
  test.skip(
    ['firefox', 'tablet-touch', 'desktop-touch-chromium'].includes(testInfo.project.name),
    'synthetic TouchEvent pan/pinch sequencing is not reliable in this project',
  )

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
