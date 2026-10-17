import { expect, test } from '../fixtures'
import {
  createApiView,
  createPlacedElement,
  createConnector,
  createTag,
  currentViewId,
  getElement,
  gotoView,
  listElements,
  listConnectors,
  listPlacements,
  nodeByName,
  pasteTextOnCanvas,
  uniqueName,
} from '../helpers/vieweditor'

async function pasteMermaid(page: Parameters<typeof currentViewId>[0], source: string) {
  await pasteTextOnCanvas(page, source)
}

function placementX(placement: Awaited<ReturnType<typeof listPlacements>>[number]) {
  return Math.round(placement.positionX ?? placement.position_x ?? 0)
}

function placementY(placement: Awaited<ReturnType<typeof listPlacements>>[number]) {
  return Math.round(placement.positionY ?? placement.position_y ?? 0)
}

test('selection bulk bar aligns distributes and removes selected placements', async ({ page }) => {
  const prefix = uniqueName('Bulk Select')
  const view = await createApiView(page, `${prefix} Diagram`)
  await gotoView(page, view.id)

  await pasteMermaid(page, `flowchart LR
  A[${prefix} A] --> B[${prefix} B]
  B --> C[${prefix} C]`)

  await expect(page.getByTestId('vieweditor-selection-bulk-bar')).toContainText('3 selected')
  await page.getByTestId('selection-bulk-align-left').click()

  await expect.poll(async () => {
    const placements = (await listPlacements(page, view.id)).filter((placement) => placement.name.startsWith(prefix))
    return new Set(placements.map(placementX)).size
  }).toBe(1)

  await page.getByTestId('selection-bulk-distribute-vertical').click()
  await expect.poll(async () => {
    const placements = (await listPlacements(page, view.id))
      .filter((placement) => placement.name.startsWith(prefix))
      .sort((left, right) => placementY(left) - placementY(right))
    if (placements.length !== 3) return false
    const firstGap = placementY(placements[1]) - placementY(placements[0])
    const secondGap = placementY(placements[2]) - placementY(placements[1])
    return Math.abs(firstGap - secondGap) <= 1
  }).toBe(true)

  await page.getByTestId('selection-bulk-remove').click()
  await expect.poll(async () => {
    const placements = await listPlacements(page, view.id)
    return placements.filter((placement) => placement.name.startsWith(prefix)).length
  }).toBe(0)
})

test('dragging selected elements persists every selected placement', async ({ page }) => {
  const prefix = uniqueName('Bulk Drag')
  const view = await createApiView(page, `${prefix} Diagram`)
  await gotoView(page, view.id)

  await pasteMermaid(page, `flowchart LR
  A[${prefix} A] --> B[${prefix} B]`)

  await expect(page.getByTestId('vieweditor-selection-bulk-bar')).toContainText('2 selected')
  const beforePlacements = (await listPlacements(page, view.id))
    .filter((placement) => placement.name.startsWith(prefix))
  const beforeByName = new Map(beforePlacements.map((placement) => [placement.name, {
    x: placementX(placement),
    y: placementY(placement),
  }]))

  const node = nodeByName(page, `${prefix} A`)
  const box = await node.boundingBox()
  expect(box).toBeTruthy()
  await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2)
  await page.mouse.down()
  await page.mouse.move(box!.x + box!.width / 2 + 180, box!.y + box!.height / 2 + 120, { steps: 12 })
  await page.mouse.up()

  await expect.poll(async () => {
    const afterPlacements = (await listPlacements(page, view.id))
      .filter((placement) => placement.name.startsWith(prefix))
    if (afterPlacements.length !== 2) return false
    return afterPlacements.every((placement) => {
      const before = beforeByName.get(placement.name)
      if (!before) return false
      return Math.abs(placementX(placement) - before.x) > 20 || Math.abs(placementY(placement) - before.y) > 20
    })
  }).toBe(true)
})

test('selection bulk tags apply to every selected imported element', async ({ page }) => {
  const prefix = uniqueName('Bulk Tags')
  const tag = uniqueName('selected-tag')
  await createTag(page, tag, '#F6AD55')
  const view = await createApiView(page, `${prefix} Diagram`)
  await gotoView(page, view.id)

  await pasteMermaid(page, `flowchart LR
  A[${prefix} A] --> B[${prefix} B]`)

  await expect(page.getByTestId('vieweditor-selection-bulk-bar')).toContainText('2 selected')
  await page.getByTestId('selection-bulk-tags').click()
  await page.getByTestId('tag-upsert-input').fill(tag)
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('Enter')

  await expect.poll(async () => (await listElements(page, prefix)).length).toBe(2)

  const imported = await listElements(page, prefix)
  const ids = imported.map((element) => element.id)
  await expect.poll(async () => {
    const elements = await Promise.all(ids.map((id) => getElement(page, id)))
    return elements.every((element) => element.tags?.includes(tag))
  }).toBe(true)

  await page.getByTestId(`selection-bulk-remove-tag-${tag}`).click()

  await expect.poll(async () => {
    const elements = await Promise.all(ids.map((id) => getElement(page, id)))
    return elements.some((element) => element.tags?.includes(tag))
  }).toBe(false)

  await page.reload()
  await expect(nodeByName(page, `${prefix} A`)).toBeVisible()
  await expect(nodeByName(page, `${prefix} B`)).toBeVisible()
})

test('nest selection moves internal connectors, preserves externals and focuses the new name', async ({ page }) => {
  const prefix = uniqueName('Nest')
  const view = await createApiView(page, `${prefix} Diagram`)
  const technology = {
    technology: 'Go',
    technologyLinks: [{ type: 'catalog', slug: 'go', label: 'Go', isPrimaryIcon: true }],
  }
  const a = await createPlacedElement(page, view.id, { name: `${prefix} A`, ...technology }, 100, 100)
  const b = await createPlacedElement(page, view.id, { name: `${prefix} B`, ...technology }, 500, 300)
  const outside = await createPlacedElement(page, view.id, { name: `${prefix} Outside` }, 800, 100)
  const internal = await createConnector(page, view.id, a.id, b.id, {
    label: 'Internal', description: 'Keep metadata', tags: ['nest-test'], direction: 'both',
    sourceHandle: 'right', targetHandle: 'left',
  })
  const outgoing = await createConnector(page, view.id, b.id, outside.id, { label: 'Outgoing' })
  const incoming = await createConnector(page, view.id, outside.id, a.id, { label: 'Incoming' })
  const before = await listPlacements(page, view.id)
  await gotoView(page, view.id)
  const boxes = await Promise.all([a, b].map((element) => nodeByName(page, element.name).boundingBox()))
  expect(boxes.every(Boolean)).toBe(true)
  const rects = boxes.map((box) => box!)
  await page.mouse.move(Math.min(...rects.map((box) => box.x)) - 12, Math.min(...rects.map((box) => box.y)) - 12)
  await page.mouse.down()
  await page.mouse.move(Math.max(...rects.map((box) => box.x + box.width)) + 12, Math.max(...rects.map((box) => box.y + box.height)) + 12, { steps: 15 })
  await page.mouse.up()
  await expect(page.getByTestId('vieweditor-selection-bulk-bar')).toContainText('2 selected')
  await page.getByTestId('selection-bulk-nest').click()

  const name = page.getByTestId('element-panel-name-input')
  await expect(name).toHaveValue('New Nest')
  await expect(name).toBeFocused()
  expect(await name.evaluate((input: HTMLInputElement) => [input.selectionStart, input.selectionEnd])).toEqual([0, 8])
  await expect(nodeByName(page, 'New Nest')).toBeVisible()
  await expect(nodeByName(page, a.name)).toHaveCount(0)
  await expect(nodeByName(page, b.name)).toHaveCount(0)
  const parentPlacements = await listPlacements(page, view.id)
  const nestPlacement = parentPlacements.find((placement) => placement.name === 'New Nest')!
  const nest = await getElement(page, nestPlacement.elementId)
  expect(parentPlacements).toHaveLength(2)
  // The element has a fixed width of 180 and height of 85.
  expect(Math.abs(placementX(nestPlacement) - 300)).toBeLessThanOrEqual(1)
  expect(Math.abs(placementY(nestPlacement) - 200)).toBeLessThanOrEqual(1)
  expect((await listConnectors(page, 0)).filter((connector) => connector.viewId === view.id)).toEqual([outgoing, incoming])

  const workspace = await page.request.post('/api/diag.v1.WorkspaceService/GetWorkspace', { data: { parentId: view.id } })
  expect(workspace.ok()).toBeTruthy()
  const workspaceJson = await workspace.json()
  const childViewId = Number(workspaceJson.views.find((child: { ownerElementId?: number }) => child.ownerElementId === nest.id).id)
  const nestedPlacements = await listPlacements(page, childViewId)
  expect(nestedPlacements.map((placement) => placement.name).sort()).toEqual([a.name, b.name].sort())
  for (const placement of nestedPlacements) {
    const original = before.find((candidate) => candidate.name === placement.name)!
    expect([placementX(placement), placementY(placement)]).toEqual([placementX(original), placementY(original)])
  }
  const childConnectors = await listConnectors(page, childViewId)
  expect(childConnectors).toHaveLength(1)
  expect(childConnectors[0]).toMatchObject({ ...internal, viewId: childViewId, updatedAt: expect.any(String) })
  const nestElement = await getElement(page, nest.id)
  expect(nestElement).toMatchObject({ ...technology, logoUrl: '/icons/go.svg' })

  await name.fill(`${prefix} Renamed`)
  await name.press('Tab')
  await expect.poll(async () => (await getElement(page, nest.id)).name).toBe(`${prefix} Renamed`)
  await page.reload()
  await expect(nodeByName(page, `${prefix} Renamed`)).toBeVisible()
  await gotoView(page, childViewId)
  await expect(nodeByName(page, a.name)).toBeVisible()
  await expect(nodeByName(page, b.name)).toBeVisible()
})
