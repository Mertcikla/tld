import { expect, test } from '../../fixtures'
import {
  addNodeWithKeyboard,
  createDiagram,
  createAndLoadDiagramWithNodes,
  expectPlacement,
  listPlacements,
  removeNodeFromPanel,
  uniqueName,
} from '../../helpers/vieweditor'


test('adds a node with the C shortcut and removes it from the element panel', async ({ page }) => {
  await createDiagram(page, uniqueName('Keyboard CRUD Diagram'))
  const nodeName = await addNodeWithKeyboard(page, uniqueName('Keyboard CRUD Node'))
  await expectPlacement(page, nodeName, true)

  await removeNodeFromPanel(page, nodeName)
  await expectPlacement(page, nodeName, false)
})

for (const selectedCount of [1, 2]) {
  test(`arrow keys persist positions for ${selectedCount} selected node(s) after reload`, async ({ page }) => {
    const { diagram, elements } = await createAndLoadDiagramWithNodes(page, selectedCount + 1, 'Keyboard Move')
    const nodes = elements.map((element) => page.locator(`.react-flow__node[data-id="${element.id}"]`))
    const before = await listPlacements(page, diagram.id)
    const selectionModifier = await page.evaluate(() => navigator.userAgent.includes('Mac') ? 'Meta' : 'Control')

    await nodes[0].focus()
    await page.keyboard.press('Enter')
    for (const node of nodes.slice(1, selectedCount)) {
      await node.focus()
      await page.keyboard.press(`${selectionModifier}+Enter`)
    }
    await expect(page.locator('.react-flow__node.selected')).toHaveCount(selectedCount)
    await nodes[0].focus()

    // Cover a single 5px nudge and a multi-selection burst with Shift's larger step.
    const keys = selectedCount === 1
      ? ['ArrowRight']
      : ['ArrowRight', 'ArrowRight', 'ArrowRight', 'ArrowLeft', 'ArrowUp', 'Shift+ArrowDown']
    for (const key of keys) {
      const transform = await nodes[0].evaluate((el) => (el as HTMLElement).style.transform)
      await page.keyboard.press(key, { delay: 50 })
      await expect.poll(() => nodes[0].evaluate((el) => (el as HTMLElement).style.transform)).not.toBe(transform)
    }

    const delta = selectedCount === 1 ? { x: 5, y: 0 } : { x: 10, y: 15 }
    for (const [index, element] of elements.entries()) {
      const initial = before.find((placement) => placement.elementId === element.id)!
      const expected = {
        x: initial.positionX! + (index < selectedCount ? delta.x : 0),
        y: initial.positionY! + (index < selectedCount ? delta.y : 0),
      }
      await expect.poll(async () => {
        const saved = (await listPlacements(page, diagram.id)).find((placement) => placement.elementId === element.id)!
        return { x: saved.positionX, y: saved.positionY }
      }).toEqual(expected)
    }

    const transforms = await Promise.all(nodes.map((node) => node.evaluate((el) => (el as HTMLElement).style.transform)))
    await page.reload()
    for (const [index, node] of nodes.entries()) {
      await expect(node).toBeVisible()
      await expect.poll(() => node.evaluate((el) => (el as HTMLElement).style.transform)).toBe(transforms[index])
    }
  })
}
