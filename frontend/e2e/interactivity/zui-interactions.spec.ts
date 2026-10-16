import { expect, test } from '../fixtures'
import {
  createApiView,
  createConnector,
  createElement,
  createPlacedElement,
  uniqueName,
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
    await expect.poll(async () => stateDelta(await zuiViewState(page), beforeMiddlePan)).toBeGreaterThan(0)

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

test('highlights connector hover, anchors its popover near the pointer, and supports nested connectors', async ({ page }) => {
  const { root, parent, sibling, child, childSource, childTarget } = await createNestedZuiFixture(page)
  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await waitForZuiReady(page)
  await waitForStableZuiCamera(page)

  const { connector } = await zuiScreenGeometry(page, {
    connector: { sourceElementId: parent.id, targetElementId: sibling.id },
  }) as { connector: { sourcePoint: { x: number; y: number }; targetPoint: { x: number; y: number } } }
  const pointer = {
    x: connector.sourcePoint.x + (connector.targetPoint.x - connector.sourcePoint.x) * 0.25,
    y: connector.sourcePoint.y,
  }
  const canvas = page.locator('canvas')
  const pixelNearConnector = () => canvas.evaluate((element: HTMLCanvasElement, point) => {
    const rect = element.getBoundingClientRect()
    const dpr = element.width / rect.width
    const ctx = element.getContext('2d')!
    return Array.from(ctx.getImageData((point.x - rect.left) * dpr, (point.y - rect.top + 1.5) * dpr, 1, 1).data)
  }, pointer)
  const before = await pixelNearConnector()
  await page.mouse.move(pointer.x, pointer.y)
  await expect(page.getByTestId('zui-hover-popover')).toBeVisible()
  await expect(page.getByTestId('zui-hover-popover')).toContainText('Parent Link')
  await expect(canvas).toHaveCSS('cursor', 'pointer')
  await expect.poll(pixelNearConnector).not.toEqual(before)
  const anchor = await page.getByTestId('zui-hover-anchor').boundingBox()
  expect(anchor).not.toBeNull()
  expect(Math.abs(anchor!.x - pointer.x)).toBeLessThan(2)
  expect(Math.abs(anchor!.y - pointer.y)).toBeLessThan(2)

  await page.getByTestId('zui-hover-popover').hover()
  await expect(page.getByRole('link', { name: `Go to ${sibling.name}` })).toBeVisible()

  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${child.id}`)
  await waitForZuiReady(page)
  await waitForStableZuiCamera(page)
  const nested = await zuiScreenGeometry(page, {
    connector: { sourceElementId: childSource.id, targetElementId: childTarget.id },
  }) as { connector: { midPoint: { x: number; y: number } } }
  await page.mouse.move(nested.connector.midPoint.x, nested.connector.midPoint.y)
  await expect(page.getByTestId('zui-hover-popover')).toBeVisible()
  await expect(page.getByTestId('zui-hover-popover')).toContainText('Child Link')
  await expect(page.getByRole('link', { name: `Go to ${childTarget.name}` })).toHaveAttribute('href', `/views/${child.id}?element=${childTarget.id}`)
})

test('element popovers keep hover while crossing a connector behind the gap', async ({ page }) => {
  const { root, parent } = await createNestedZuiFixture(page)
  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await waitForZuiReady(page)
  await waitForStableZuiCamera(page)
  const { node } = await zuiScreenGeometry(page, { nodeElementId: parent.id }) as {
    node: { x: number; y: number; width: number; height: number }
  }
  // The connector leaves the middle of this card's right edge and passes
  // directly behind the gap into its right-aligned popover.
  const start = { x: node.x + node.width - 4, y: node.y + node.height / 2 }
  await page.mouse.move(start.x, start.y)
  const popover = page.getByTestId('zui-hover-popover')
  await expect(popover).toContainText(parent.name)
  await expect.poll(() => popover.evaluate((element) => new DOMMatrixReadOnly(getComputedStyle(element).transform).a)).toBe(1)
  const initialAnchor = await page.getByTestId('zui-hover-anchor').boundingBox()
  const initialPopup = await popover.boundingBox()
  const action = page.getByRole('link', { name: 'Open in Editor' })
  const button = await action.boundingBox()
  const destination = { x: button!.x + button!.width / 2, y: button!.y + button!.height / 2 }
  for (let step = 1; step <= 20; step += 1) {
    await page.mouse.move(start.x + (destination.x - start.x) * step / 20, start.y + (destination.y - start.y) * step / 20)
    await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
    await expect(action).toBeVisible()
  }
  expect(await page.getByTestId('zui-hover-anchor').boundingBox()).toEqual(initialAnchor)
  expect(await popover.boundingBox()).toEqual(initialPopup)
  // Moving away still releases hover, allowing another object to be targeted.
  const box = await canvasBox(page)
  await page.mouse.move(box.x + 4, box.y + box.height - 4)
  await expect(popover).toBeHidden()
  await page.mouse.move(start.x, start.y)
  await expect(popover).toContainText(parent.name)
  await expect(action).toHaveAttribute('href', `/views/${root.id}?element=${parent.id}`)
  await action.click()
  await expect(page).toHaveURL(new RegExp(`/views/${root.id}(?:\\?|$)`))
})

test('connector popovers stay anchored and remain reachable across the safe hover corridor', async ({ page }) => {
  const { root, parent, sibling } = await createNestedZuiFixture(page)
  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await waitForZuiReady(page)
  await waitForStableZuiCamera(page)
  const { connector } = await zuiScreenGeometry(page, {
    connector: { sourceElementId: parent.id, targetElementId: sibling.id },
  }) as { connector: { sourcePoint: { x: number; y: number }; targetPoint: { x: number; y: number } } }
  const start = {
    x: connector.sourcePoint.x + (connector.targetPoint.x - connector.sourcePoint.x) * 0.25,
    y: connector.sourcePoint.y,
  }
  await page.mouse.move(start.x, start.y)
  const popover = page.getByTestId('zui-hover-popover')
  await expect(popover).toContainText('Parent Link')
  await expect.poll(() => popover.evaluate((element) => new DOMMatrixReadOnly(getComputedStyle(element).transform).a)).toBe(1)
  const anchor = page.getByTestId('zui-hover-anchor')
  const initialAnchor = await anchor.boundingBox()
  const initialPopup = await popover.boundingBox()
  await page.mouse.move(start.x + (connector.targetPoint.x - connector.sourcePoint.x) * 0.3, start.y, { steps: 8 })
  expect(await anchor.boundingBox()).toEqual(initialAnchor)
  expect(await popover.boundingBox()).toEqual(initialPopup)

  const action = page.getByRole('link', { name: `Go to ${sibling.name}` })
  const button = await action.boundingBox()
  const destination = { x: button!.x + button!.width / 2, y: button!.y + button!.height / 2 }
  // Move slowly through the gap, longer than the dismissal grace period.
  // Hovering must keep the original target instead of chasing the pointer.
  for (let step = 1; step <= 20; step += 1) {
    await page.mouse.move(start.x + (destination.x - start.x) * step / 20, start.y + (destination.y - start.y) * step / 20)
    await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => requestAnimationFrame(() => resolve()))))
  }
  await expect(popover).toContainText('Parent Link')
  expect(await anchor.boundingBox()).toEqual(initialAnchor)
  await expect(action).toHaveAttribute('href', `/views/${root.id}?element=${sibling.id}`)
  await action.click()
  await expect(page).toHaveURL(new RegExp(`/views/${root.id}(?:\\?|$)`))
})

test('clicking an element jumps into its contents while dragging only pans', async ({ page }, testInfo) => {
  const { root, parent, childSource } = await createNestedZuiFixture(page)
  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await waitForZuiReady(page)
  await waitForStableZuiCamera(page)
  const geometry = () => zuiScreenGeometry(page, { nodeElementId: parent.id }) as Promise<{
    node: { x: number; y: number; width: number; height: number }
  }>
  const before = await zuiViewState(page)
  const initial = (await geometry()).node
  await dispatchZuiMousePan(page,
    { x: initial.x + initial.width / 2, y: initial.y + initial.height / 2 },
    { x: initial.x + initial.width / 2 + 30, y: initial.y + initial.height / 2 + 20 },
  )
  await waitForStableZuiCamera(page)
  expect((await zuiViewState(page)).zoom).toBeCloseTo(before.zoom, 5)

  const node = (await geometry()).node
  const point = { x: node.x + node.width / 2, y: node.y + node.height / 2 }
  if (testInfo.project.use.hasTouch) await page.touchscreen.tap(point.x, point.y)
  else await page.mouse.click(point.x, point.y)
  const box = await canvasBox(page)
  const revealThreshold = Math.min(640, Math.max(200, box.width * 0.4))
  await expect.poll(async () => (await geometry()).node.width).toBeGreaterThan(revealThreshold)
  await waitForStableZuiCamera(page)
  expect((await zuiViewState(page)).zoom).toBeGreaterThan(before.zoom)
  const { node: child } = await zuiScreenGeometry(page, { nodeElementId: childSource.id }) as {
    node: { x: number; y: number; width: number; height: number }
  }
  await page.mouse.move(child.x + child.width / 2, child.y + child.height / 2)
  await expect(page.getByTestId('zui-hover-popover')).toContainText(childSource.name)
})

test('anchors off-view connector popovers at either hovered end of the route', async ({ page }) => {
  const root = await createApiView(page, uniqueName('Proxy Hover Root'))
  const parent = await createPlacedElement(page, root.id, { name: uniqueName('Parent') }, 120, 140)
  const target = await createPlacedElement(page, root.id, { name: uniqueName('Target') }, 720, 140)
  const child = await createApiView(page, uniqueName('Proxy Hover Child'), parent.id)
  const source = await createPlacedElement(page, child.id, { name: uniqueName('Source') }, 120, 140)
  await createConnector(page, root.id, source.id, target.id, { label: 'Off-view call' })
  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await waitForZuiReady(page)
  await waitForStableZuiCamera(page)
  const { connector } = await zuiScreenGeometry(page, {
    connector: { sourceElementId: parent.id, targetElementId: target.id },
  }) as { connector: { sourcePoint: { x: number; y: number }; targetPoint: { x: number; y: number } } }
  const box = await canvasBox(page)
  for (const progress of [0.2, 0.8]) {
    await page.mouse.move(box.x + 5, box.y + 5)
    await expect(page.getByTestId('zui-hover-popover')).toBeHidden()
    const pointer = {
      x: connector.sourcePoint.x + (connector.targetPoint.x - connector.sourcePoint.x) * progress,
      y: connector.sourcePoint.y,
    }
    await page.mouse.move(pointer.x, pointer.y)
    const popover = page.getByTestId('zui-hover-popover')
    await expect(popover).toBeVisible()
    await expect(popover).toContainText('Off-View Connector')
    await expect.poll(() => popover.evaluate((element) => new DOMMatrixReadOnly(getComputedStyle(element).transform).a)).toBe(1)
    const anchor = await page.getByTestId('zui-hover-anchor').boundingBox()
    expect(Math.abs(anchor!.x - pointer.x)).toBeLessThan(2)
    expect(Math.abs(anchor!.y - pointer.y)).toBeLessThan(2)
    // Popper can flip start/end alignment or shift inward on narrow screens.
    // Measure placement and bounds together after its positioning update.
    await expect.poll(() => popover.evaluate((element, reference) => {
      const popup = element.getBoundingClientRect()
      const placement = element.closest('[data-popper-placement]')?.getAttribute('data-popper-placement')
      const alignedLeft = placement?.endsWith('-end') ? reference.x + reference.width - popup.width : reference.x
      const expectedLeft = Math.max(8, Math.min(alignedLeft, document.documentElement.clientWidth - popup.width - 8))
      return Math.abs(popup.x - expectedLeft)
    }, { x: anchor!.x, width: anchor!.width })).toBeLessThan(2)
  }
})

test('shows connected unplaced elements inside a synthetic Explore boundary', async ({ page }) => {
  const root = await createApiView(page, uniqueName('Unplaced Explore'))
  const source = await createPlacedElement(page, root.id, { name: uniqueName('Placed Source') })
  const target = await createElement(page, { name: uniqueName('Unplaced Target'), technology: 'Go' })
  await createConnector(page, root.id, source.id, target.id, { label: 'Calls unplaced service' })
  await page.goto('/views?view=explore&debugZuiTest=1')
  await waitForZuiReady(page)
  await page.getByRole('button', { name: 'Fit View', exact: true }).click()
  await waitForStableZuiCamera(page)
  const geometry = () => zuiScreenGeometry(page, { nodeElementId: target.id }) as Promise<{
    node: { x: number; y: number; width: number; height: number; diagramId: number }
  }>
  const initial = (await geometry()).node
  expect(initial.diagramId).toBeLessThan(0)
  // Other tests add roots to this workspace; Fit View can reach its minimum zoom
  // before the synthetic boundary fits on screen. Pan to the actual hover target.
  const box = await canvasBox(page)
  const center = { x: box.x + box.width / 2, y: box.y + box.height / 2 }
  await dispatchZuiMousePan(page, center, {
    x: center.x * 2 - (initial.x + initial.width / 2),
    y: center.y * 2 - (initial.y + initial.height / 2),
  }, { button: 'middle' })
  await waitForStableZuiCamera(page)
  const node = (await geometry()).node
  await page.mouse.move(node.x + node.width / 2, node.y + node.height / 2)
  await expect(page.getByTestId('zui-hover-popover')).toContainText(target.name)
  await expect(page.getByTestId('zui-hover-popover').getByRole('button', { name: 'Open in Editor', exact: true })).toBeDisabled()
})

test('handles one-finger touch pan, two-finger pinch, and transition back to touch pan', async ({ page }, testInfo) => {
  test.skip(
    ['firefox', 'webkit', 'mobile-chrome', 'tablet-touch', 'desktop-touch-chromium'].includes(testInfo.project.name),
    'synthetic TouchEvent pan/pinch sequencing is not reliable in this project',
  )

  const { root } = await createNestedZuiFixture(page)
  await page.goto(`/views?view=explore&debugZuiTest=1&focus=${root.id}`)
  await waitForZuiReady(page)
  const box = await canvasBox(page)
  const center = { x: box.x + box.width / 2, y: box.y + box.height / 2 }

  await dispatchZuiTouchPan(page, { x: center.x - 70, y: center.y - 20 }, { x: center.x + 30, y: center.y + 60 })
  await expect.poll(async () => (await zuiLastInteraction(page))?.mode).toBe('one-finger-pan')

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
  await expect.poll(async () => (await zuiLastInteraction(page))?.mode).toBe('one-finger-pan')
})

test('lets native-wheel overlays scroll without moving the ZUI camera', async ({ page }, testInfo) => {
  test.skip(
    testInfo.project.name === 'tablet-touch',
    'synthetic wheel overlay scrolling is not stable on the tablet touch project',
  )

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
