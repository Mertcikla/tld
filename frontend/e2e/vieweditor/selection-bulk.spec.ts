import { expect, test } from '../fixtures'
import {
  createApiView,
  createTag,
  currentViewId,
  getElement,
  gotoView,
  listElements,
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

  await page.getByRole('button', { name: 'Remove', exact: true }).click()

  await expect.poll(async () => {
    const elements = await Promise.all(ids.map((id) => getElement(page, id)))
    return elements.some((element) => element.tags?.includes(tag))
  }).toBe(false)

  await page.reload()
  await expect(nodeByName(page, `${prefix} A`)).toBeVisible()
  await expect(nodeByName(page, `${prefix} B`)).toBeVisible()
})
