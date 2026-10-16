import type { Page } from '@playwright/test'
import { expect, test } from '../fixtures'
import { createApiView, createConnector, createPlacedElement, gotoView, listConnectors, listPlacements, nodeByName, uniqueName } from '../helpers/vieweditor'

const mime = 'application/x-tldiagram-view-selection+json'

async function selectionClipboard(page: Page, type: 'copy' | 'cut') {
  return page.evaluate(({ type, mime }) => {
    const data = new DataTransfer()
    const event = new Event(type, { bubbles: true, cancelable: true })
    Object.defineProperty(event, 'clipboardData', { value: data })
    document.querySelector('[data-testid="vieweditor-canvas"]')!.dispatchEvent(event)
    return data.getData(mime)
  }, { type, mime })
}

async function pasteSelection(page: Page, payload: string) {
  await page.evaluate(({ payload, mime }) => {
    const data = new DataTransfer()
    data.setData(mime, payload)
    const event = new Event('paste', { bubbles: true, cancelable: true })
    Object.defineProperty(event, 'clipboardData', { value: data })
    document.querySelector('[data-testid="vieweditor-canvas"]')!.dispatchEvent(event)
  }, { payload, mime })
}

async function storedConnectors(page: Page, viewId: number) {
  return (await listConnectors(page, 0)).filter((connector) => connector.viewId === viewId)
}

test('cut and paste moves internal relationships and refreshes the view hierarchy', async ({ page }) => {
  const prefix = uniqueName('Selection move')
  const source = await createApiView(page, prefix)
  const names = ['CLI', 'Code Intelligence', 'Local Server', 'Web UI', 'Persistence', 'Cloud', 'YAML', 'Import']
  const elements = []
  for (const [i, name] of names.entries()) {
    elements.push(await createPlacedElement(page, source.id, { name: `${prefix} ${name}` }, (i % 4) * 280, Math.floor(i / 4) * 200))
  }
  const parent = await createPlacedElement(page, source.id, { name: `${prefix} tld` }, 300, 450)
  const destination = await createApiView(page, `${prefix} tld`, parent.id)
  for (const element of elements.slice(0, 4)) {
    const child = await createApiView(page, `${element.name} detail`, element.id)
    const implementation = await createPlacedElement(page, child.id, { name: `${element.name} implementation` })
    await createConnector(page, child.id, implementation.id, elements[4].id, { label: 'uses persistence' })
  }
  for (const [a, b] of [[0, 4], [0, 5], [0, 6], [0, 7], [1, 4], [1, 5], [2, 5], [2, 6], [3, 7], [4, 6], [4, 7]]) {
    await createConnector(page, source.id, elements[a].id, elements[b].id, { label: `${a} calls ${b}` })
  }
  const originalIDs = (await listConnectors(page, source.id)).map((connector) => connector.id).sort((a, b) => a - b)
  const boundary = await createConnector(page, source.id, elements[0].id, parent.id, { label: 'crosses selection boundary' })
  await gotoView(page, source.id)
  await expect(page.locator('.react-flow__node-elementNode')).toHaveCount(9)
  const boxes = await Promise.all(elements.map((element) => nodeByName(page, element.name).boundingBox()))
  expect(boxes.every(Boolean)).toBe(true)
  const rects = boxes.map((box) => box!)
  await page.mouse.move(Math.min(...rects.map((box) => box.x)) - 12, Math.min(...rects.map((box) => box.y)) - 12)
  await page.mouse.down()
  await page.mouse.move(Math.max(...rects.map((box) => box.x + box.width)) + 12, Math.max(...rects.map((box) => box.y + box.height)) + 12, { steps: 15 })
  await page.mouse.up()
  await expect(page.getByTestId('vieweditor-selection-bulk-bar')).toContainText('8 selected')
  const payload = await selectionClipboard(page, 'cut')
  expect(JSON.parse(payload).connectors).toHaveLength(11)
  await expect(page.getByText('Selection cut', { exact: true }).first()).toBeVisible()
  await expect.poll(async () => (await listPlacements(page, source.id)).length).toBe(1)
  expect((await storedConnectors(page, source.id)).map((connector) => connector.id)).toEqual([boundary.id])

  await page.locator(`.react-flow__node[data-id="${parent.id}"]`).click({ position: { x: 30, y: 25 } })
  await page.keyboard.press('s')
  await expect(page).toHaveURL(new RegExp(`/views/${destination.id}$`))
  await expect(page.locator('.react-flow__node-elementNode')).toHaveCount(0)

  await pasteSelection(page, payload)
  await expect(page.getByText('Pasted selection', { exact: true }).first()).toBeVisible()
  await expect(page.locator('.react-flow__node-elementNode')).toHaveCount(8)
  for (const element of elements.slice(0, 4)) {
    await expect(page.locator('.react-flow__node-contextNeighborNode').filter({ hasText: element.name })).toHaveCount(0)
  }
  expect(await listPlacements(page, source.id)).toHaveLength(1)
  expect((await storedConnectors(page, source.id)).map((connector) => connector.id)).toEqual([boundary.id])
  const moved = await storedConnectors(page, destination.id)
  expect(moved).toHaveLength(originalIDs.length)
  expect(new Set(moved.map((connector) => connector.label)).size).toBe(11)
  await page.reload()
  await expect(page.locator('.react-flow__node-elementNode')).toHaveCount(8)
  for (const element of elements.slice(0, 4)) {
    await expect(page.locator('.react-flow__node-contextNeighborNode').filter({ hasText: element.name })).toHaveCount(0)
  }
})

test('copy preserves parallel connectors without placement enrichment duplicates', async ({ page }) => {
  const prefix = uniqueName('Selection copy')
  const source = await createApiView(page, prefix)
  const target = await createApiView(page, `${prefix} destination`)
  const a = await createPlacedElement(page, source.id, { name: `${prefix} A` }, 100, 100)
  const b = await createPlacedElement(page, source.id, { name: `${prefix} B` }, 500, 100)
  for (let i = 0; i < 2; i++) await createConnector(page, source.id, a.id, b.id, { label: 'calls' })
  await gotoView(page, source.id)
  await nodeByName(page, a.name).focus()
  await page.keyboard.press('ControlOrMeta+a')
  await expect(page.getByTestId('vieweditor-selection-bulk-bar')).toContainText('2 selected')
  const payload = await selectionClipboard(page, 'copy')
  expect(JSON.parse(payload).connectors).toHaveLength(2)
  await gotoView(page, target.id)
  await pasteSelection(page, payload)
  await expect(page.getByText('Pasted selection', { exact: true }).first()).toBeVisible()
  expect(await listPlacements(page, source.id)).toHaveLength(2)
  expect(await listConnectors(page, source.id)).toHaveLength(2)
  expect(await listConnectors(page, target.id)).toHaveLength(2)
})
