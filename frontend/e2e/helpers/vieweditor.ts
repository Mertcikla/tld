import { expect, type Locator, type Page } from '@playwright/test'

export const onboardingStorage = {
  editor: 'diagrameditor_tutorial_v1_core',
  viewGrid: 'viewgrid_tutorial_v2_core',
  shown: 'onboarding_shown',
}

export async function prepareStorage(page: Page) {
  await page.addInitScript((keys) => {
    localStorage.setItem(keys.editor, '1')
    localStorage.setItem(keys.viewGrid, '1')
    localStorage.setItem(keys.shown, '1')
    localStorage.setItem('diag:libraryOpen', 'true')
    localStorage.setItem('diag:explorerOpen', 'true')
    localStorage.setItem('diag:snapToGrid', 'false')
  }, onboardingStorage)
}

export function uniqueName(prefix: string) {
  return `${prefix} ${Date.now()} ${Math.random().toString(36).slice(2, 8)}`
}

export async function createDiagram(page: Page, name = uniqueName('E2E Diagram')) {
  await page.goto('/views?view=hierarchy')
  await page.getByTestId('views-new-diagram-button').click()
  await page.getByTestId('views-new-diagram-name-input').fill(name)
  await page.getByTestId('views-create-diagram-submit').click()
  await expect(page).toHaveURL(/\/views\/\d+$/)
  await expect(page.getByTestId('vieweditor-canvas')).toBeVisible()
  return { name, id: currentViewId(page) }
}

export function currentViewId(page: Page) {
  const match = page.url().match(/\/views\/(\d+)/)
  if (!match) throw new Error(`Expected /views/:id URL, got ${page.url()}`)
  return Number(match[1])
}

export function nodeByName(page: Page, name: string): Locator {
  return page.getByTestId('vieweditor-node').filter({ hasText: name })
}

export async function addNodeWithToolbar(page: Page, name = uniqueName('Toolbar Node')) {
  await page.getByTestId('vieweditor-toolbar-add-element').click()
  await confirmInlineNewElement(page, name)
  await expect(nodeByName(page, name)).toBeVisible()
  return name
}

export async function addNodeWithKeyboard(page: Page, name = uniqueName('Keyboard Node')) {
  await page.getByTestId('vieweditor-canvas').click()
  await page.keyboard.press('c')
  await confirmInlineNewElement(page, name)
  await expect(nodeByName(page, name)).toBeVisible()
  return name
}

export async function addNodeWithCanvasContextMenu(page: Page, name = uniqueName('Context Node')) {
  const pane = page.locator('.react-flow__pane')
  const box = await pane.boundingBox()
  if (!box) throw new Error('React Flow pane is not visible')

  await page.mouse.click(box.x + box.width * 0.52, box.y + box.height * 0.42, { button: 'right' })
  const addElementButtons = page.getByRole('button', { name: 'Add Element C' })
  await expect(addElementButtons).toHaveCount(2)
  await addElementButtons.first().click()
  await confirmInlineNewElement(page, name)
  await expect(nodeByName(page, name)).toBeVisible()
  return name
}

export async function addExistingNodeWithInlineSearch(page: Page, name: string) {
  await page.getByTestId('vieweditor-toolbar-add-element').click()
  const input = page.getByTestId('inline-element-adder-input')
  await input.fill(name)
  await expect(page.getByTestId('inline-element-adder-existing-option').filter({ hasText: name }).first()).toBeVisible()
  await page.keyboard.press('ArrowDown')
  await page.keyboard.press('Enter')
  await expect(nodeByName(page, name)).toBeVisible()
}

export async function confirmInlineNewElement(page: Page, name: string) {
  const input = page.getByTestId('inline-element-adder-input')
  await expect(input).toBeVisible()
  await input.fill(name)
  await expect(page.getByTestId('inline-element-adder-create-option').filter({ hasText: name }).first()).toBeVisible()
  await input.press('Enter')
}

export async function deleteSelectedNodeWithKeyboard(page: Page, name: string) {
  await nodeByName(page, name).click()
  await page.keyboard.press('Delete')
  await expect(nodeByName(page, name)).toHaveCount(0)
}

export async function removeNodeFromPanel(page: Page, name: string) {
  await nodeByName(page, name).click()
  await expect(page.getByTestId('element-panel')).toBeVisible()
  await page.getByTestId('element-panel-remove').click()
  await expect(nodeByName(page, name)).toHaveCount(0)
}

export async function removeSelectedNodeWithBackspace(page: Page, name: string) {
  await nodeByName(page, name).click()
  await page.keyboard.press('Backspace')
  await expect(nodeByName(page, name)).toHaveCount(0)
}

export async function listPlacements(page: Page, viewId = currentViewId(page)) {
  const response = await page.request.post('/api/diag.v1.WorkspaceService/ListPlacements', {
    data: { viewId },
  })
  expect(response.ok()).toBeTruthy()
  const json = await response.json()
  return (json.placements ?? []) as Array<{ id: number; viewId: number; elementId: number; name: string }>
}

export async function expectPlacement(page: Page, name: string, visible: boolean, viewId = currentViewId(page)) {
  await expect.poll(async () => {
    const placements = await listPlacements(page, viewId)
    return placements.some((placement) => placement.name === name)
  }).toBe(visible)
}
