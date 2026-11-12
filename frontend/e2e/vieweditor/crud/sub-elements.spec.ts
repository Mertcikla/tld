import type { Locator, Page } from '@playwright/test'
import { expect, test } from '../../fixtures'
import { addPlacement, createApiView, createConnector, createPlacedElement, expectConnector, gotoView, handleLocator, listConnectors, locatorCenter, nodeByName } from '../../helpers/vieweditor'

async function symbolLeft(locator: Locator) {
  const bounds = await locator.boundingBox()
  if (!bounds) throw new Error('Missing symbol text')
  return { x: bounds.x + 4, y: bounds.y + bounds.height / 2 }
}

async function resizeCard(page: Page, name: string, currentWidth: number, deltaWidth: number, deltaHeight: number) {
  const node = nodeByName(page, name)
  const rect = await node.boundingBox()
  const grip = await node.getByTestId('sub-element-resize').boundingBox()
  if (!rect || !grip) throw new Error('Missing card resize grip')
  const zoom = rect.width / currentWidth
  const x = grip.x + grip.width / 2, y = grip.y + grip.height / 2
  await page.mouse.move(x, y)
  await page.mouse.down()
  await page.mouse.move(x + deltaWidth * zoom, y + deltaHeight * zoom, { steps: 8 })
  await page.mouse.up()
  await expect(node.getByTestId('sub-element-resize')).toHaveAttribute('aria-busy', 'false')
}

async function previewSize(page: Page, viewId: number, elementId: number, width: number, height: number) {
  await page.addInitScript(({ key, size }) => { if (!localStorage.getItem(key)) localStorage.setItem(key, JSON.stringify(size)) }, {
    key: `tld:source-preview-size:${viewId}:${elementId}`, size: { width, height },
  })
}

test('protobuf class links show source previews without class editing or stored presentation fields', async ({ page }) => {
  const content = 'syntax = "proto3";\n\nmessage User {\n  string id = 1;\n  string name = 2;\n}\n'
  await page.route('**/GetWorktreeSource', route => route.fulfill({ json: { content, path: 'users.proto' } }))
  const view = await createApiView(page, 'Protobuf previews')
  const element = await createPlacedElement(page, view.id, { name: 'User proto', kind: 'class', repo: '/test/repo', filePath: 'users.proto' }, 100, 100)
  await gotoView(page, view.id)
  const node = nodeByName(page, 'User proto')
  await expect(node).toHaveAttribute('data-detail-level', 'overview')
  await node.getByTestId('vieweditor-node-source-link').click()
  await expect(node).toHaveAttribute('data-detail-level', 'code')
  await expect(node.getByText('message', { exact: true })).toHaveCSS('color', 'rgb(198, 120, 221)')
  await expect(node.getByTestId('sub-symbol-region')).toHaveCount(1)
  await node.getByTestId('sub-symbol-region').hover()
  await expect(node.locator('[data-symbol-port="true"][data-handlepos="right"]')).toHaveAttribute('data-handleid', 'right-symbol-message%3AUser')
  await node.getByTestId('sub-element-header').getByText('User proto', { exact: true }).click()
  await expect(page.getByLabel('Notation', { exact: true })).toHaveCount(0)
  await expect(page.getByRole('button', { name: 'Add member', exact: true })).toHaveCount(0)
  await page.keyboard.press('Escape')
  await resizeCard(page, 'User proto', 520, 80, 40)
  await expect(node).toHaveCSS('width', '600px')
  await expect(node).toHaveCSS('height', '400px')
  await page.keyboard.press('ControlOrMeta+z')
  await expect(node).toHaveCSS('width', '520px')
  await page.keyboard.press('ControlOrMeta+Shift+z')
  await expect(node).toHaveCSS('width', '600px')
  const record = await page.request.post('/api/diag.v1.WorkspaceService/GetElement', { data: { elementId: element.id } })
  expect((await record.json()).element).not.toHaveProperty('subElement')
  await node.getByRole('button', { name: 'Collapsed symbols layout', exact: true }).click()
  await expect(node.locator('.sub-symbol-text')).toHaveText('message User')
  await expect(node.getByTestId('sub-element-resize')).toBeVisible()
  await expect(node.locator('[data-testid^="sub-symbol-ellipsis-"]')).toHaveCount(0)
  await page.reload()
  await expect(node).toHaveAttribute('data-detail-level', 'outline')
  await node.getByRole('button', { name: 'Full layout', exact: true }).click()
  await expect(node).toHaveCSS('width', '600px')
  await expect(node.getByText('message', { exact: true })).toBeVisible()
})

test('resizing previews preserves the chosen layout without updating element data', async ({ page }) => {
  await page.route('**/GetWorktreeSource', route => route.fulfill({ json: { content: 'message User { string id = 1; }' } }))
  const updates: string[] = []
  page.on('request', request => { if (request.url().endsWith('/UpdateElement')) updates.push(request.url()) })
  const view = await createApiView(page, 'Preview resizing')
  const element = await createPlacedElement(page, view.id, { name: 'Resizable proto', kind: 'service', repo: '/test/repo', filePath: 'users.proto' }, 100, 100)
  await gotoView(page, view.id)
  const node = nodeByName(page, element.name)
  await node.getByRole('button', { name: 'Collapsed symbols layout', exact: true }).click()
  await resizeCard(page, element.name, 400, 200, 120)
  await expect(node).toHaveAttribute('data-detail-level', 'outline')
  await expect(node).toHaveCSS('width', '600px')
  await node.getByRole('button', { name: 'Full layout', exact: true }).click()
  await resizeCard(page, element.name, 600, -300, -200)
  await expect(node).toHaveAttribute('data-detail-level', 'code')
  await expect(node).toHaveCSS('width', '300px')
  await expect(node).toHaveCSS('height', '160px')
  expect(updates).toEqual([])
})

test('protobuf symbol ports connect through source areas and share overflow handles', async ({ page }) => {
  const docs: Record<string, string> = {}
  await page.route('**/GetWorktreeSource', route => route.fulfill({ json: { content: docs[route.request().postDataJSON().filePath] } }))
  const view = await createApiView(page, 'Protobuf connections')
  const create = async (name: string, count: number, x: number) => {
    docs[`${name}.proto`] = Array.from({ length: count }, (_, index) => `message M${index} { string id = 1; }`).join('\n')
    const element = await createPlacedElement(page, view.id, { name, kind: 'class', repo: '/test/repo', filePath: `${name}.proto` }, x, 100)
    await previewSize(page, view.id, element.id, 520, 240)
    return element
  }
  const source = await create('Source', 20, 100), target = await create('Target', 2, 900)
  await gotoView(page, view.id)
  const sourceNode = nodeByName(page, 'Source'), targetNode = nodeByName(page, 'Target')
  for (const node of [sourceNode, targetNode]) await node.getByTestId('vieweditor-node-source-link').click()
  const port = sourceNode.locator('[data-handleid="right-symbol-message%3AM0"]')
  const region = sourceNode.getByTestId('sub-symbol-region').first()
  await page.mouse.move(10, 10)
  await expect(port).toHaveCSS('opacity', '0')
  await expect(region).toHaveCSS('background-color', 'rgba(0, 0, 0, 0)')
  await expect(region).toHaveCSS('border-color', 'rgba(0, 0, 0, 0)')
  await sourceNode.getByTestId('sub-symbol-region').first().hover()
  await expect(region).toHaveAttribute('data-highlighted', 'true')
  await expect(port).toHaveCSS('opacity', '1')
  await expect(sourceNode.getByTestId('sub-symbol-stem')).toHaveCount(2)
  await page.mouse.move(10, 10)
  await expect(port).toHaveCSS('opacity', '0')
  const start = await locatorCenter(port), destination = await symbolLeft(targetNode.locator('.sub-symbol-text').first())
  await page.mouse.move(start.x, start.y)
  await page.mouse.down()
  await expect(targetNode.locator('[data-handleid="right-symbol-message%3AM1"]')).toHaveCSS('opacity', '1')
  await page.mouse.move(destination.x, destination.y, { steps: 12 })
  await page.mouse.up()
  await expectConnector(page, { sourceElementId: source.id, targetElementId: target.id, sourceHandle: 'right-symbol-message%3AM0', targetHandle: 'left-symbol-message%3AM0' }, true, view.id)
  await page.mouse.move(10, 10)
  await expect(port).toHaveCSS('opacity', '1')
  await expect(region).toHaveAttribute('data-highlighted', 'true')
  await expect(sourceNode.getByTestId('sub-symbol-stem')).toHaveCount(1)
  const [connector] = await listConnectors(page, view.id)
  const pickup = (endpoint: 'source' | 'target') => page.locator(`[data-testid="vieweditor-node-reconnect-zone"][data-edge-id="${connector.id}"][data-endpoint="${endpoint}"]`)
  const dragEndpoint = async (endpoint: 'source' | 'target', destination: { x: number; y: number }) => {
    const zone = pickup(endpoint)
    await expect(zone).toBeVisible()
    const start = await locatorCenter(zone)
    await page.mouse.move(start.x, start.y)
    await expect(page.getByTestId('vieweditor-canvas')).toHaveAttribute('data-reconnect-hover', 'true')
    await page.mouse.down()
    // Reveal all target symbols before the pointer reaches the preview, including
    // unused symbols while an existing connector is being moved.
    for (const index of [0, 1]) await expect(targetNode.locator(`[data-handleid="right-symbol-message%3AM${index}"]`)).toHaveCSS('opacity', '1')
    await expect(targetNode.getByTestId('sub-symbol-region').nth(1)).toHaveAttribute('data-highlighted', 'true')
    await page.mouse.move(destination.x, destination.y, { steps: 12 })
    await page.mouse.up()
  }
  await dragEndpoint('target', await symbolLeft(targetNode.locator('.sub-symbol-text').nth(1)))
  await expectConnector(page, { id: connector.id, targetHandle: 'left-symbol-message%3AM1' }, true, view.id)
  await page.mouse.move(10, 10)
  await expect(targetNode.locator('[data-handleid="right-symbol-message%3AM0"]')).toHaveCSS('opacity', '0')
  await targetNode.getByRole('button', { name: 'Collapsed symbols layout', exact: true }).click()
  await sourceNode.locator('.source-preview-scroll').evaluate(element => element.scrollTop = element.scrollHeight)
  await expect(sourceNode.getByTestId('sub-symbol-ellipsis-above')).toContainText('1 connection')
  await expect(sourceNode.locator('[data-symbol-overflow="above"][data-symbol-port="true"]')).toHaveCount(1)
  await expect(sourceNode.getByTestId('sub-symbol-stem')).toHaveCount(0)
  // Offscreen endpoints use the shared ellipsis port's pickup area.
  await dragEndpoint('source', await symbolLeft(sourceNode.locator('.sub-symbol-text').last()))
  await expectConnector(page, { id: connector.id, sourceHandle: 'right-symbol-message%3AM19', targetHandle: 'left-symbol-message%3AM1' }, true, view.id)
  await sourceNode.getByRole('button', { name: 'Collapsed symbols layout', exact: true }).click()
  await sourceNode.locator('.source-preview-scroll').evaluate(element => element.scrollTop = element.scrollHeight)
  await expect(pickup('source')).toBeVisible()
  await sourceNode.getByRole('button', { name: 'Card layout', exact: true }).click()
  // Card mode retains semantic endpoints and uses the same pickup interaction.
  await dragEndpoint('target', await symbolLeft(targetNode.locator('.sub-symbol-text').first()))
  await expectConnector(page, { id: connector.id, sourceHandle: 'right-symbol-message%3AM19', targetHandle: 'left-symbol-message%3AM0' }, true, view.id)
  await expect(pickup('source')).toBeVisible()
})

test('top overflow clicks reveal symbols beneath the band and keep progressing upward', async ({ page }) => {
  const content = Array.from({ length: 40 }, (_, index) => `message M${index} { string id = 1; }`).join('\n')
  await page.route('**/GetWorktreeSource', route => route.fulfill({ json: { content } }))
  const view = await createApiView(page, 'Overflow navigation')
  const element = await createPlacedElement(page, view.id, { name: 'Overflow proto', kind: 'class', repo: '/test/repo', filePath: 'overflow.proto' }, 100, 100)
  await previewSize(page, view.id, element.id, 520, 240)
  await gotoView(page, view.id)
  const node = nodeByName(page, 'Overflow proto')
  await node.getByTestId('vieweditor-node-source-link').click()
  const scroller = node.locator('.source-preview-scroll')
  const banner = node.getByTestId('sub-symbol-ellipsis-above')
  for (const [layout, rowHeight] of [['Full layout', 15], ['Collapsed symbols layout', 23]] as const) {
    await node.getByRole('button', { name: layout, exact: true }).click()
    // M10's start is within the overlay, despite its row starting below scrollTop.
    const initialTop = rowHeight * 10 - 1
    await scroller.evaluate((element, top) => { element.scrollTop = top }, initialTop)
    await expect(banner).toContainText('11 symbols above')
    await banner.click()
    await expect.poll(() => scroller.evaluate(element => element.scrollTop)).toBeLessThan(initialTop)
    await expect(node.locator('[data-handleid="right-symbol-message%3AM10"]')).not.toHaveAttribute('data-symbol-overflow')
    const previousTop = await scroller.evaluate(element => element.scrollTop)
    await banner.focus()
    await page.keyboard.press('Enter')
    await expect.poll(() => scroller.evaluate(element => element.scrollTop)).toBeLessThan(previousTop)
    await expect(node.locator('[data-handleid="right-symbol-message%3AM9"]')).not.toHaveAttribute('data-symbol-overflow')
  }
})

test('activated symbol bodies drag connectors with a ghost from the nearest side', async ({ page }) => {
  await page.route('**/GetWorktreeSource', route => route.fulfill({ json: { content: 'message First { string id = 1; }\nmessage Second { string id = 1; }' } }))
  const view = await createApiView(page, 'Symbol body drags')
  const source = await createPlacedElement(page, view.id, { name: 'Drag source', kind: 'class', repo: '/test/repo', filePath: 'source.proto' }, 500, 100)
  const left = await createPlacedElement(page, view.id, { name: 'Left consumer', kind: 'service' }, 100, 100)
  const right = await createPlacedElement(page, view.id, { name: 'Right symbols', kind: 'class', repo: '/test/repo', filePath: 'target.proto' }, 1200, 100)
  for (const element of [source, right]) await previewSize(page, view.id, element.id, 520, 240)
  await gotoView(page, view.id)
  const node = nodeByName(page, source.name), target = nodeByName(page, right.name)
  for (const preview of [node, target]) await preview.getByTestId('vieweditor-node-source-link').click()
  const ghost = page.getByTestId('alt-connector-connector')
  for (const layout of ['Full layout', 'Collapsed symbols layout']) {
    await node.getByRole('button', { name: layout, exact: true }).click()
    const body = node.locator('.sub-symbol-text').first()
    await page.mouse.move(10, 10)
    await body.hover()
    await page.mouse.down()
    await expect(ghost).toHaveCount(0)
    await page.mouse.up()
    await expect(node).toHaveAttribute('data-preview-interaction', 'ready')
    await expect(body).toHaveCSS('cursor', 'crosshair')
    await page.mouse.down()
    await expect(ghost).toBeVisible()
    const targetPort = target.locator('[data-handleid="left-symbol-message%3ASecond"]')
    await expect(targetPort).toHaveCSS('opacity', '1')
    const destination = await locatorCenter(targetPort)
    await page.mouse.move(destination.x, destination.y, { steps: 12 })
    await expect(node.getByTestId('sub-symbol-region').first()).toHaveAttribute('data-highlighted', 'true')
    await expect(node.getByTestId('sub-symbol-region').first()).toHaveAttribute('data-active', 'true')
    const sourcePort = node.locator('[data-handleid="right-symbol-message%3AFirst"]')
    await expect.poll(async () => {
      const origin = await ghost.locator('path').evaluate(element => {
        const point = (element as SVGPathElement).getPointAtLength(0)
        return { x: point.x, y: point.y }
      })
      const port = await locatorCenter(sourcePort)
      return Math.hypot(origin.x - port.x, origin.y - port.y)
    }).toBeLessThan(2)
    await page.mouse.up()
    await expect(ghost).toHaveCount(0)
    await expectConnector(page, { sourceElementId: source.id, targetElementId: right.id, sourceHandle: 'right-symbol-message%3AFirst', targetHandle: 'left-symbol-message%3ASecond' }, true, view.id)

    await body.hover()
    await expect(node).toHaveAttribute('data-preview-interaction', 'ready')
    await page.mouse.down()
    const leftDestination = await locatorCenter(nodeByName(page, left.name))
    await page.mouse.move(leftDestination.x, leftDestination.y, { steps: 12 })
    await expect(ghost).toBeVisible()
    await page.mouse.up()
    await expectConnector(page, { sourceElementId: source.id, targetElementId: left.id, sourceHandle: 'left-symbol-message%3AFirst' }, true, view.id)
  }
  // Cancelling the gesture removes its ghost without creating a connector.
  const count = (await listConnectors(page, view.id)).length
  await node.locator('.sub-symbol-text').nth(1).hover()
  await expect(node).toHaveAttribute('data-preview-interaction', 'ready')
  await page.mouse.down()
  await expect(ghost).toBeVisible()
  await page.mouse.move(10, 10)
  await expect(node.getByTestId('sub-symbol-region').nth(1)).toHaveAttribute('data-highlighted', 'true')
  await page.keyboard.press('Escape')
  await expect(ghost).toHaveCount(0)
  await expect(node.getByTestId('sub-symbol-region').nth(1)).not.toHaveAttribute('data-highlighted')
  await page.mouse.up()
  expect(await listConnectors(page, view.id)).toHaveLength(count)
})

test('symbol handles reveal as click-connect targets in full and collapsed layouts', async ({ page }) => {
  await page.route('**/GetWorktreeSource', route => route.fulfill({ json: { content: 'message First { string id = 1; }\nmessage Second { string id = 1; }' } }))
  const view = await createApiView(page, 'Visible symbol targets')
  const source = await createPlacedElement(page, view.id, { name: 'Regular source', kind: 'service' }, 100, 100)
  const target = await createPlacedElement(page, view.id, { name: 'Symbol target', kind: 'service', repo: '/test/repo', filePath: 'target.proto' }, 700, 100)
  const rightSource = await createPlacedElement(page, view.id, { name: 'Right source', kind: 'service' }, 1400, 100)
  await gotoView(page, view.id)
  const sourceNode = nodeByName(page, source.name), targetNode = nodeByName(page, target.name)
  await targetNode.getByTestId('vieweditor-node-source-link').click()
  const port = targetNode.locator('[data-handleid="right-symbol-message%3ASecond"]')
  await targetNode.getByRole('button', { name: 'Collapsed symbols layout', exact: true }).click()
  const rightNode = nodeByName(page, 'Right source')
  await rightNode.hover()
  const dragStart = await locatorCenter(handleLocator(page, rightSource.id, 'left-2'))
  const body = await symbolLeft(targetNode.locator('.sub-symbol-text').first())
  await page.mouse.move(dragStart.x, dragStart.y)
  await page.mouse.down()
  await expect(targetNode.getByTestId('sub-symbol-region').nth(1)).toHaveAttribute('data-highlighted', 'true')
  await page.mouse.move(body.x, body.y, { steps: 12 })
  await page.mouse.up()
  await expectConnector(page, { sourceElementId: rightSource.id, targetElementId: target.id, targetHandle: 'right-symbol-message%3AFirst' }, true, view.id)

  for (const layout of ['Full layout', 'Collapsed symbols layout']) {
    await targetNode.getByRole('button', { name: layout, exact: true }).click()
    await sourceNode.click()
    await page.keyboard.press('e')
    await page.mouse.move(10, 10)
    await expect(port).toHaveCSS('opacity', '1')
    await expect(port).toHaveAttribute('data-symbol-port', 'true')
    await expect(targetNode.getByTestId('sub-symbol-region').nth(1)).toHaveAttribute('data-highlighted', 'true')
    await page.keyboard.press('Escape')
    await expect(port).toHaveCSS('opacity', '0')
    await expect(targetNode.getByTestId('sub-symbol-region').nth(1)).not.toHaveAttribute('data-highlighted')
  }
  for (const [origin, side] of [[sourceNode, 'left'], [nodeByName(page, 'Right source'), 'right']] as const) {
    await origin.click()
    await page.keyboard.press('e')
    await page.getByTestId('element-panel').getByTestId('panel-close').click()
    const region = targetNode.getByTestId('sub-symbol-region').first()
    const bounds = await region.boundingBox()
    if (!bounds) throw new Error('Missing symbol boundary')
    await page.mouse.click(bounds.x + 8, bounds.y + bounds.height / 2)
    await expectConnector(page, { targetElementId: target.id, targetHandle: `${side}-symbol-message%3AFirst` }, true, view.id)
  }
  await sourceNode.click()
  await page.keyboard.press('e')
  await page.getByTestId('element-panel').getByTestId('panel-close').click()
  await port.click()
  await expectConnector(page, { sourceElementId: source.id, targetElementId: target.id, targetHandle: 'right-symbol-message%3ASecond' }, true, view.id)
})

test('source previews reuse regular pickup zones for old handles and selected symbol aliases', async ({ page }) => {
  await page.route('**/GetWorktreeSource', route => route.fulfill({ json: { content: 'message First { string id = 1; }\nmessage Second { string id = 1; }' } }))
  const view = await createApiView(page, 'Source endpoint pickup')
  const source = await createPlacedElement(page, view.id, { name: 'Pickup source', kind: 'service', repo: '/test/repo', filePath: 'pickup.proto' }, 100, 100)
  const target = await createPlacedElement(page, view.id, { name: 'Regular target', kind: 'service' }, 900, 100)
  const ordinary = await createConnector(page, view.id, source.id, target.id, { sourceHandle: 'right-2', targetHandle: 'left-2' })
  const alias = await createConnector(page, view.id, source.id, target.id, { sourceHandle: 'right-line-1', targetHandle: 'left-0' })
  const symbol = await createConnector(page, view.id, source.id, target.id, { sourceHandle: 'right-symbol-message%3AFirst', targetHandle: 'left-4' })
  await gotoView(page, view.id)
  const node = nodeByName(page, source.name)
  const zone = (id: number) => node.locator(`[data-testid="vieweditor-node-reconnect-zone"][data-edge-id="${id}"][data-endpoint="source"]`)
  await expect(zone(ordinary.id)).toBeVisible()
  await expect(zone(alias.id)).toBeVisible()
  await node.getByTestId('vieweditor-node-source-link').click()
  await expect(zone(ordinary.id)).toBeVisible()
  const dragToSecond = async (id: number) => {
    const start = await locatorCenter(zone(id)), destination = await symbolLeft(node.locator('.sub-symbol-text').nth(1))
    await page.mouse.move(start.x, start.y)
    await page.mouse.down()
    await page.mouse.move(destination.x, destination.y, { steps: 12 })
    await page.mouse.up()
    await expectConnector(page, { id, sourceHandle: 'right-symbol-message%3ASecond' }, true, view.id)
  }
  await dragToSecond(ordinary.id)
  await expect(zone(alias.id)).toHaveAttribute('data-handle-id', 'right-line-1')
  await page.getByTestId(`rf__edge-${symbol.id}`).click({ force: true })
  await expect(zone(symbol.id)).toHaveAttribute('data-selected', 'true')
  await expect(zone(symbol.id)).toHaveCSS('width', '44px')
  await expect(zone(alias.id)).toHaveCount(0)
  await dragToSecond(symbol.id)
  await expectConnector(page, { id: alias.id, sourceHandle: 'right-line-1', targetHandle: 'left-0' }, true, view.id)
})

test('selecting a symbol connector reveals both endpoints without locking manual scroll', async ({ page }) => {
  const content = Array.from({ length: 40 }, (_, index) => `message M${index} { string id = 1; }`).join('\n')
  await page.route('**/GetWorktreeSource', route => route.fulfill({ json: { content } }))
  const view = await createApiView(page, 'Reveal selected symbols')
  const source = await createPlacedElement(page, view.id, { name: 'Reveal source', kind: 'service', repo: '/test/repo', filePath: 'source.proto' }, 100, 100)
  const target = await createPlacedElement(page, view.id, { name: 'Reveal target', kind: 'service', repo: '/test/repo', filePath: 'target.proto' }, 900, 100)
  const connector = await createConnector(page, view.id, source.id, target.id, { sourceHandle: 'left-symbol-message%3AM20', targetHandle: 'right-symbol-message%3AM30' })
  for (const element of [source, target]) await previewSize(page, view.id, element.id, 520, 240)
  await gotoView(page, view.id)
  const nodes = [nodeByName(page, source.name), nodeByName(page, target.name)]
  for (const node of nodes) await node.getByRole('button', { name: 'Full layout', exact: true }).click()
  await page.getByTestId(`rf__edge-${connector.id}`).click({ force: true })
  for (const [index, node] of nodes.entries()) {
    const port = node.locator(`[data-handleid="${index ? 'right' : 'left'}-symbol-message%3AM${index ? 30 : 20}"]`)
    await expect(port).not.toHaveAttribute('data-symbol-overflow')
    await expect.poll(() => node.locator('.source-preview-scroll').evaluate(element => element.scrollTop)).toBeGreaterThan(0)
    await node.locator('.source-preview-scroll').evaluate(element => element.scrollTop = 0)
    await expect(port).toHaveAttribute('data-symbol-overflow', 'below')
    await expect(node.locator('.source-preview-scroll')).toHaveJSProperty('scrollTop', 0)
    await node.getByRole('button', { name: 'Collapsed symbols layout', exact: true }).click()
    await expect(port).not.toHaveAttribute('data-symbol-overflow')
    await expect.poll(() => node.locator('.source-preview-scroll').evaluate(element => element.scrollTop)).toBeGreaterThan(0)
  }
})

test('repo-linked service elements replace the panel with inline source and header navigation', async ({ page }) => {
  let sourceRequests = 0
  const editorRequests: Record<string, unknown>[] = []
  await page.route('https://raw.githubusercontent.com/acme/preview/**', route => {
    sourceRequests++
    return route.fulfill({ contentType: 'text/plain', headers: { 'access-control-allow-origin': '*' }, body: '-- context\nCREATE TABLE users (\n  id TEXT\n);' })
  })
  await page.context().route('https://github.com/acme/preview/**', route => route.fulfill({ contentType: 'text/html', body: '<p>GitHub target</p>' }))
  await page.route('**/api/ready', route => route.fulfill({ json: { ok: true, capabilities: { editor: true, repositories: true, watch: true } } }))
  await page.route('**/api/editor/open', route => {
    editorRequests.push(route.request().postDataJSON())
    return route.fulfill({ json: { ok: true } })
  })
  const view = await createApiView(page, 'Inline source migration')
  const response = await page.request.post('/api/diag.v1.WorkspaceService/CreateElement', { data: {
    name: 'Linked service', kind: 'service', repo: 'https://github.com/acme/preview', branch: 'feature/inline', filePath: 'users.sql#L2-L4', logoUrl: '/normal.svg', tags: ['source-preview'],
  } })
  expect(response.ok()).toBeTruthy()
  const { element } = await response.json()
  await addPlacement(page, view.id, element.id, 100, 100)
  await gotoView(page, view.id)
  const node = nodeByName(page, 'Linked service')
  await expect(node).toHaveAttribute('data-detail-level', 'overview')
  await expect(node).toHaveCSS('width', '180px')
  expect(sourceRequests).toBe(0)
  await node.getByTestId('vieweditor-node-source-link').click()
  await expect(node).toHaveAttribute('data-detail-level', 'code')
  await expect(node.getByText('CREATE', { exact: true })).toBeVisible()
  await expect(page.getByTestId('code-preview-panel')).toHaveCount(0)
  await expect(page.getByTestId('element-panel')).toHaveCount(0)
  const header = node.getByTestId('sub-element-header')
  await expect(header.getByText('service', { exact: true })).toBeVisible()
  const [github] = await Promise.all([page.waitForEvent('popup'), header.getByRole('button', { name: 'Open in GitHub', exact: true }).click()])
  await expect(github).toHaveURL('https://github.com/acme/preview/blob/feature%2Finline/users.sql#L2-L4')
  await github.close()
  await header.getByRole('button', { name: /Open in (VS Code|Zed)/ }).click()
  await expect.poll(() => editorRequests[0]).toMatchObject({ repo: 'https://github.com/acme/preview', file_path: 'users.sql', line: 2 })
  await resizeCard(page, 'Linked service', 520, 80, 0)
  await expect.poll(async () => {
    const response = await page.request.post('/api/diag.v1.WorkspaceService/GetElement', { data: { elementId: element.id } })
    return (await response.json()).element
  }).toMatchObject({ kind: 'service', logoUrl: '/normal.svg', tags: ['source-preview'] })
  await page.keyboard.press('ControlOrMeta+z')
  await expect(node).toHaveCSS('width', '520px')
  await expect(node.getByAltText('technology icon')).toHaveAttribute('src', '/normal.svg')
  await page.keyboard.press('ControlOrMeta+Shift+z')
  await expect(node).toHaveCSS('width', '600px')
  await expect(node.getByAltText('technology icon')).toHaveAttribute('src', '/normal.svg')
  await node.getByRole('button', { name: 'Collapsed symbols layout', exact: true }).click()
  await expect(header.getByRole('button', { name: 'Open in GitHub', exact: true })).toBeVisible()
  await expect(header.getByRole('button', { name: /Open in (VS Code|Zed)/ })).toBeVisible()
  await expect(node.locator('.sub-symbol-text')).toHaveText('CREATE TABLE users (')
  await page.reload()
  await expect(node).toHaveAttribute('data-detail-level', 'outline')
  await expect(node.getByTestId('sub-element-header').getByText('service', { exact: true })).toBeVisible()
})

test('connected source stems track horizontal scrolling and fold into ellipsis rows vertically', async ({ page }) => {
  const content = [`CREATE TABLE users (${Array.from({ length: 40 }, (_, i) => `field${i} TEXT`).join(', ')});`, '\t-- indented 数据🙂', ...Array.from({ length: 58 }, (_, i) => `-- line ${i + 3}`)].join('\n')
  await page.route('**/GetWorktreeSource', route => route.fulfill({ json: { content, path: 'scroll.sql' } }))
  const view = await createApiView(page, 'Scrolling connected symbols')
  const response = await page.request.post('/api/diag.v1.WorkspaceService/CreateElement', { data: {
    name: 'scroll.sql', kind: 'code', repo: '/test/repo', filePath: 'scroll.sql', language: 'SQL',
  } })
  expect(response.ok()).toBeTruthy()
  const { element } = await response.json()
  await previewSize(page, view.id, element.id, 600, 320)
  const target = await createPlacedElement(page, view.id, { name: 'Consumer', kind: 'service' }, 900, 100)
  await addPlacement(page, view.id, element.id, 100, 100)
  const sourceHandle = `right-symbol-${encodeURIComponent(':scroll.sql')}`
  await createConnector(page, view.id, element.id, target.id, { sourceHandle, targetHandle: 'left-2' })
  await gotoView(page, view.id)
  const node = nodeByName(page, 'scroll.sql')
  const stems = node.getByTestId('sub-symbol-stem')
  await expect(node.getByTestId('sub-symbol-region')).toHaveCount(1)
  await expect(stems).toHaveCount(0)
  const scroller = node.locator('.source-preview-scroll')
  const expectEnclosedCode = async () => {
    const overflow = await node.evaluate(element => {
      const region = element.querySelector('[data-testid="sub-symbol-region"]')!.getBoundingClientRect()
      return [...element.querySelectorAll('.sub-symbol-text')].filter(text => {
        if (!text.textContent?.trim()) return false
        const range = document.createRange()
        range.selectNodeContents(text)
        const bounds = range.getBoundingClientRect()
        return bounds.right > region.right - 1 || bounds.top < region.top || bounds.bottom > region.bottom
      }).map(text => text.textContent)
    })
    expect(overflow).toEqual([])
  }
  await expectEnclosedCode()
  await scroller.evaluate(element => { element.scrollLeft = 120 })
  await expectEnclosedCode()
  await expect(stems).toHaveCount(0)
  const endScroll = await scroller.evaluate(element => {
    element.scrollLeft = element.scrollWidth - element.clientWidth
    return element.scrollLeft
  })
  await expectEnclosedCode()
  await expect(stems).toHaveCount(1)
  const originalX = await stems.evaluate(element => (element as SVGPathElement).getPointAtLength(0).x)
  expect(600 - originalX).toBeGreaterThanOrEqual(64)
  await scroller.evaluate((element, scroll) => { element.scrollLeft = scroll - 4 }, endScroll)
  await expect.poll(() => stems.evaluate(element => (element as SVGPathElement).getPointAtLength(0).x)).toBeCloseTo(originalX + 4, 0)
  await scroller.evaluate(element => { element.scrollLeft = 0; element.scrollTop = 120 })
  await expect(node.getByTestId('sub-symbol-ellipsis-above')).toContainText('1 connection')
  await expect(node.locator('[data-symbol-overflow="above"][data-symbol-port="true"]')).toHaveCount(1)
  await expect(stems).toHaveCount(0)
  // The external connector must follow the shared ellipsis port, then return to its symbol.
  const portCenter = await locatorCenter(node.locator(`[data-handleid="${sourceHandle}"]`))
  await expect.poll(async () => {
    const point = await page.locator('.react-flow__edge-path').first().evaluate(element => {
      const path = element as SVGPathElement
      const start = path.getPointAtLength(0)
      const screen = new DOMPoint(start.x, start.y).matrixTransform(path.getScreenCTM()!)
      return { x: screen.x, y: screen.y }
    })
    return Math.hypot(point.x - portCenter.x, point.y - portCenter.y)
  }).toBeLessThan(2)
  await scroller.evaluate((element, scroll) => { element.scrollTop = 0; element.scrollLeft = scroll }, endScroll)
  await expect(stems).toHaveCount(1)
  await expect(stems).toHaveAttribute('d', /^M [\d.]+ [\d.]+ H [\d.]+$/)

  // Before scroll events reach React, native scrolling must not move the visible code
  // independently of its connector overlay (the compositor can paint that intermediate state).
  const immediateError = await node.evaluate(element => {
    const scroller = element.querySelector<HTMLElement>('.source-preview-scroll')!
    const region = element.querySelector('[data-testid="sub-symbol-region"]')!
    const stem = element.querySelector<SVGPathElement>('[data-testid="sub-symbol-stem"]')!
    const originalRegion = region.getBoundingClientRect()
    const from = stem.getPointAtLength(0)
    const originalStem = new DOMPoint(from.x, from.y).matrixTransform(stem.getScreenCTM()!)
    scroller.scrollLeft -= 16
    scroller.scrollTop = 6
    const afterRegion = region.getBoundingClientRect()
    const afterFrom = stem.getPointAtLength(0)
    const afterStem = new DOMPoint(afterFrom.x, afterFrom.y).matrixTransform(stem.getScreenCTM()!)
    return Math.max(Math.abs((afterStem.x - afterRegion.right) - (originalStem.x - originalRegion.right)),
      Math.abs((afterStem.y - afterRegion.top) - (originalStem.y - originalRegion.top)))
  })
  expect(immediateError).toBeLessThan(1)
  await scroller.evaluate((element, scroll) => { element.scrollTop = 0; element.scrollLeft = scroll }, endScroll)
  await expect(stems).toHaveCount(1)

  // Exercise real animation and viewport updates, without the fixture's transition overrides.
  await page.locator('style[data-tld-e2e="disable-animations"]').evaluateAll(styles => styles.forEach(style => style.remove()))
  await node.evaluate(element => {
    const trace = { running: true, errors: [] as number[], frames: 0 }
    ;(window as unknown as { symbolRenderTrace: typeof trace }).symbolRenderTrace = trace
    const sample = () => {
      if (!trace.running) return
      const stem = element.querySelector<SVGPathElement>('[data-testid="sub-symbol-stem"]')
      const region = element.querySelector('[data-testid="sub-symbol-region"]')
      const port = stem && element.querySelector(`[data-handleid="${CSS.escape(stem.dataset.symbolHandle!)}"]`)
      if (stem?.isConnected && region && port) {
        const scale = element.getBoundingClientRect().width / parseFloat(getComputedStyle(element).width)
        const matrix = stem.getScreenCTM()!
        const from = stem.getPointAtLength(0), to = stem.getPointAtLength(stem.getTotalLength())
        const a = new DOMPoint(from.x, from.y).matrixTransform(matrix), b = new DOMPoint(to.x, to.y).matrixTransform(matrix)
        const shape = region.getBoundingClientRect(), handle = port.getBoundingClientRect()
        const edge = document.querySelector<SVGPathElement>('.react-flow__edge-path')!
        const edgePoint = edge.getPointAtLength(0)
        const external = new DOMPoint(edgePoint.x, edgePoint.y).matrixTransform(edge.getScreenCTM()!)
        trace.errors.push(Math.max(Math.abs(a.x - shape.right), Math.abs(a.y - b.y), Math.abs(b.x - handle.x - handle.width / 2), Math.abs(b.y - handle.y - handle.height / 2), Math.abs(external.y - b.y)) / scale)
        trace.frames++
      }
      requestAnimationFrame(sample)
    }
    requestAnimationFrame(sample)
  })
  for (const offset of [20, 16, 12, 8, 4, 0]) {
    await scroller.evaluate((element, left) => { element.scrollLeft = left }, endScroll - offset)
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => resolve(null))))
  }
  const body = await node.boundingBox()
  if (!body) throw new Error('Missing source card')
  await page.mouse.move(body.x + 100, body.y + 130)
  await expect(node).toHaveAttribute('data-preview-interaction', 'ready', { timeout: 5000 })
  await page.mouse.wheel(-4, 0)
  await expect.poll(() => scroller.evaluate(element => element.scrollLeft)).toBeCloseTo(endScroll - 4, 0)
  for (const top of [2, 4, 6, 8, 0]) {
    await scroller.evaluate((element, top) => { element.scrollTop = top }, top)
    await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => resolve(null))))
  }
  await page.keyboard.press('+')
  await page.keyboard.press('-')
  // Refocus can reveal the start of a horizontally scrolled line in Firefox.
  await scroller.evaluate((element, left) => { element.scrollLeft = left }, endScroll)
  await expect(stems).toHaveCount(1)
  const pane = await page.locator('.react-flow__pane').boundingBox()
  if (!pane) throw new Error('Missing canvas pane')
  const panStart = { x: pane.x + 50, y: pane.y + pane.height - 120 }
  await page.mouse.move(panStart.x, panStart.y)
  await page.mouse.down({ button: 'middle' })
  await page.mouse.move(panStart.x + 50, panStart.y + 30, { steps: 10 })
  await expect(page.locator('[data-canvas-moving]')).toHaveCount(1)
  await expect(stems).toHaveCSS('animation-play-state', 'paused')
  await page.mouse.up({ button: 'middle' })
  await expect(page.locator('[data-canvas-moving]')).toHaveCount(0)
  const trace = await page.evaluate(() => {
    const trace = (window as unknown as { symbolRenderTrace: { running: boolean; errors: number[]; frames: number } }).symbolRenderTrace
    trace.running = false
    return { frames: trace.frames, maxError: Math.max(...trace.errors) }
  })
  expect(trace.frames).toBeGreaterThan(10)
  expect(trace.maxError).toBeLessThan(2)
})


test('source-linked layouts use regular element curve geometry', async ({ page }) => {
  await page.route('**/GetWorktreeSource', route => route.fulfill({ json: { content: 'message User { string id = 1; }' } }))
  const view = await createApiView(page, 'Identical card routing')
  const regularSource = await createPlacedElement(page, view.id, { name: 'Regular curve source', kind: 'service' }, 100, 100)
  const regularTarget = await createPlacedElement(page, view.id, { name: 'Regular curve target', kind: 'service' }, 100, 500)
  const linkedSource = await createPlacedElement(page, view.id, { name: 'Linked curve source', kind: 'class', repo: '/test/repo', filePath: 'source.proto' }, 700, 100)
  const linkedTarget = await createPlacedElement(page, view.id, { name: 'Linked curve target', kind: 'class', repo: '/test/repo', filePath: 'target.proto' }, 700, 500)
  const handles = { sourceHandle: 'left-2', targetHandle: 'left-2' }
  const regular = await createConnector(page, view.id, regularSource.id, regularTarget.id, handles)
  const linked = await createConnector(page, view.id, linkedSource.id, linkedTarget.id, handles)
  for (const element of [linkedSource, linkedTarget]) await previewSize(page, view.id, element.id, 600, 360)
  await gotoView(page, view.id)
  await expect(nodeByName(page, linkedSource.name)).toHaveAttribute('data-detail-level', 'overview')
  const normalized = async (id: number) => {
    const path = await page.getByTestId(`rf__edge-${id}`).locator('.react-flow__edge-path').first().getAttribute('d')
    const values = path!.match(/-?\d*\.?\d+(?:e[-+]?\d+)?/gi)!.map(Number)
    return values.map((value, index) => Math.round((value - values[index % 2]) * 1000) / 1000)
  }
  await expect.poll(() => normalized(linked.id)).toEqual(await normalized(regular.id))
  for (const layout of ['Collapsed symbols layout', 'Full layout']) {
    for (const element of [linkedSource, linkedTarget]) await nodeByName(page, element.name).getByRole('button', { name: layout, exact: true }).click()
    await page.keyboard.press('f')
    await expect.poll(async () => (await normalized(linked.id))[2]).toBe(-90)
  }
})


test('source previews share regular zoom navigation in collapsed and full headers', async ({ page }) => {
  await page.route('**/GetWorktreeSource', route => route.fulfill({ json: { content: 'message User { string id = 1; }' } }))
  const root = await createApiView(page, 'Source navigation root')
  const owner = await createPlacedElement(page, root.id, { name: 'Navigation owner', kind: 'class', repo: '/test/repo', filePath: 'owner.proto' }, 100, 100)
  const child = await createApiView(page, 'Source navigation child', owner.id)
  await createPlacedElement(page, child.id, { name: 'Nested preview', kind: 'service', repo: '/test/repo', filePath: 'nested.proto' }, 100, 100)
  await gotoView(page, root.id)
  for (const layout of ['Collapsed symbols layout', 'Full layout']) {
    const node = nodeByName(page, owner.name)
    await node.getByRole('button', { name: layout, exact: true }).click()
    const header = node.getByTestId('sub-element-header')
    await expect(header.getByRole('button', { name: 'Reload source' })).toHaveCount(0)
    await expect(header.getByTestId('vieweditor-node-zoom-out')).toBeVisible()
    await header.getByTestId('vieweditor-node-zoom-in').click()
    await expect(page).toHaveURL(new RegExp(`/views/${child.id}$`))
    const nested = nodeByName(page, 'Nested preview')
    await nested.getByRole('button', { name: layout, exact: true }).click()
    await nested.getByTestId('sub-element-header').getByTestId('vieweditor-node-zoom-out').click()
    await expect(page).toHaveURL(new RegExp(`/views/${root.id}$`))
  }
})


test('preview gestures wait for the hover delay and display a pointer hint', async ({ page }) => {
  await page.route('**/GetWorktreeSource', route => route.fulfill({ json: { content: Array.from({ length: 80 }, (_, i) => `message M${i} { string id = 1; }`).join('\n') } }))
  const view = await createApiView(page, 'Preview hover gate')
  const element = await createPlacedElement(page, view.id, { name: 'Hover preview', kind: 'class', repo: '/test/repo', filePath: 'hover.proto' }, 100, 100)
  await previewSize(page, view.id, element.id, 520, 240)
  await gotoView(page, view.id)
  const node = nodeByName(page, element.name)
  await node.getByRole('button', { name: 'Full layout', exact: true }).click()
  const scroller = node.locator('.source-preview-scroll')
  const viewport = page.locator('.react-flow__viewport')
  await scroller.hover({ position: { x: 100, y: 50 } })
  await expect(page.getByRole('progressbar', { name: 'Activating preview interaction' })).toBeVisible()
  await expect(page.getByText('Keep hovering to interact', { exact: true })).toBeVisible()
  const before = await viewport.getAttribute('style')
  // Start by zooming in: this single-card view may already be at its minimum
  // zoom, where zooming out (and panning at the extent) correctly does nothing.
  await page.mouse.wheel(0, -120)
  await expect(viewport).not.toHaveAttribute('style', before!)
  await expect(scroller).toHaveJSProperty('scrollTop', 0)
  await scroller.hover({ position: { x: 100, y: 50 } })
  await expect(node).toHaveAttribute('data-preview-interaction', 'ready', { timeout: 5000 })
  await expect(page.getByRole('progressbar', { name: 'Activating preview interaction' })).toHaveCount(0)
  const locked = await viewport.getAttribute('style')
  await page.mouse.wheel(0, 120)
  await expect.poll(() => scroller.evaluate(element => element.scrollTop)).toBeGreaterThan(0)
  await expect(viewport).toHaveAttribute('style', locked!)
  const center = await locatorCenter(scroller)
  const top = await scroller.evaluate(element => element.scrollTop)
  await page.mouse.move(center.x, center.y)
  await page.mouse.down({ button: 'middle' })
  await page.mouse.move(center.x - 20, center.y - 30, { steps: 5 })
  await page.mouse.up({ button: 'middle' })
  await expect.poll(() => scroller.evaluate(element => element.scrollTop)).toBeGreaterThan(top)
  await expect(viewport).toHaveAttribute('style', locked!)
  await node.getByTestId('sub-element-header').hover({ position: { x: 100, y: 40 } })
  await expect(node).toHaveAttribute('data-preview-interaction', 'idle')
  await expect(page.getByRole('progressbar', { name: 'Activating preview interaction' })).toHaveCount(0)
  const headerTop = await scroller.evaluate(element => element.scrollTop)
  await page.mouse.wheel(4, 0)
  await expect(viewport).not.toHaveAttribute('style', locked!)
  await expect(scroller).toHaveJSProperty('scrollTop', headerTop)
  await node.getByRole('button', { name: 'Full layout', exact: true }).hover()
  await expect(node).toHaveAttribute('data-preview-interaction', 'idle')
  await page.mouse.move(10, 10)
  await expect(node).toHaveAttribute('data-preview-interaction', 'idle')
  await scroller.hover({ position: { x: 100, y: 50 } })
  await expect(node).toHaveAttribute('data-preview-interaction', 'waiting')
})


test('preview borders share regular element resting, hover, and selection styles', async ({ page }) => {
  await page.route('**/GetWorktreeSource', route => route.fulfill({ json: { content: 'message User { string id = 1; }' } }))
  const view = await createApiView(page, 'Shared element borders')
  const regular = await createPlacedElement(page, view.id, { name: 'Regular border', kind: 'service' }, 100, 100)
  const preview = await createPlacedElement(page, view.id, { name: 'Preview border', kind: 'class', repo: '/test/repo', filePath: 'border.proto' }, 600, 100)
  await gotoView(page, view.id)
  const card = nodeByName(page, regular.name), node = nodeByName(page, preview.name)
  const border = node.getByTestId('sub-element-container')
  const styles = (locator: Locator) => locator.evaluate(element => {
    const css = getComputedStyle(element)
    return [css.borderColor, css.borderWidth, css.borderRadius, css.boxShadow]
  })
  for (const layout of ['Collapsed symbols layout', 'Full layout']) {
    await node.getByRole('button', { name: layout, exact: true }).click()
    await page.keyboard.press('f')
    await page.getByTestId('vieweditor-canvas').click({ position: { x: 10, y: 10 } })
    await page.mouse.move(10, 10)
    await expect.poll(() => styles(border)).toEqual(await styles(card))
    await card.hover()
    const hovered = await styles(card)
    await node.getByTestId('sub-element-header').hover()
    await expect.poll(() => styles(border)).toEqual(hovered)
    await card.getByText(regular.name, { exact: true }).click()
    await page.mouse.move(10, 10)
    const selected = await styles(card)
    await node.getByTestId('sub-element-header').getByText(preview.name, { exact: true }).click()
    await page.mouse.move(10, 10)
    await expect.poll(() => styles(border)).toEqual(selected)
    await page.keyboard.press('Escape')
  }
})


test('code appearance controls resize text, toggle gutters, and persist per element', async ({ page }) => {
  await page.route('**/GetWorktreeSource', route => route.fulfill({ json: { content: 'message User {\n  string id = 1;\n}\nmessage Team { string id = 1; }' } }))
  const view = await createApiView(page, 'Code appearance')
  const element = await createPlacedElement(page, view.id, { name: 'Appearance preview', kind: 'class', repo: '/test/repo', filePath: 'appearance.proto' }, 100, 100)
  await gotoView(page, view.id)
  const node = nodeByName(page, element.name)
  await expect(node.getByRole('button', { name: 'Increase code font size' })).toHaveCount(0)
  await node.getByRole('button', { name: 'Full layout', exact: true }).click()
  const text = node.locator('.sub-symbol-text').first()
  const initialWidth = await node.getByTestId('sub-symbol-region').first().evaluate(element => element.getBoundingClientRect().width)
  await expect(text).toHaveCSS('font-size', '10px')
  await expect(node.getByTestId('source-code-line-number').first()).toBeVisible()
  await node.getByRole('button', { name: 'Increase code font size' }).click()
  await expect(text).toHaveCSS('font-size', '11px')
  await expect.poll(() => node.getByTestId('sub-symbol-region').first().evaluate(element => element.getBoundingClientRect().width)).toBeGreaterThan(initialWidth)
  await node.getByRole('button', { name: 'Toggle code line numbers' }).click()
  await expect(node.getByTestId('source-code-line-number')).toHaveCount(0)
  await node.getByRole('button', { name: 'Collapsed symbols layout', exact: true }).click()
  await expect(text).toHaveCSS('font-size', '11px')
  await node.getByRole('button', { name: 'Toggle code line numbers' }).click()
  await expect(node.getByTestId('source-code-line-number')).toHaveText(['1', '4'])
  await page.reload()
  await expect(text).toHaveCSS('font-size', '11px')
  await expect(node.getByTestId('source-code-line-number')).toHaveText(['1', '4'])
  await node.getByRole('button', { name: 'Decrease code font size' }).click()
  await expect(text).toHaveCSS('font-size', '10px')
})
