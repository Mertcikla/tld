import type { Locator, Page } from '@playwright/test'
import { expect, test } from '../fixtures'
import {
  createApiView,
  deleteView,
  getView,
  uniqueName,
} from '../helpers/vieweditor'

type ApiView = Awaited<ReturnType<typeof createApiView>>

function viewGridNode(page: Page, viewId: number): Locator {
  return page.locator(`[data-testid="views-grid-node"][data-view-id="${viewId}"]`).first()
}

function waitForUpdateView(page: Page) {
  return page.waitForResponse((response) =>
    response.url().includes('/api/diag.v1.WorkspaceService/UpdateView') &&
    response.request().method() === 'POST',
  )
}

async function openNodeMenu(page: Page, view: ApiView) {
  const search = page.getByTestId('views-search-input')
  await search.fill(view.name)
  await expect(search).toHaveValue(view.name)

  const node = viewGridNode(page, view.id)
  await expect(node).toBeVisible()
  await expect(node).toContainText(view.name)

  const menuButton = node.getByTestId('views-grid-node-menu')
  await expect(menuButton).toBeVisible()
  await expect(menuButton).toBeEnabled()
  await menuButton.click()
  await expect(page.getByTestId('views-grid-node-details')).toBeVisible()
  return node
}

test('clears jump search results from the views grid', async ({ page }) => {
  const view = await createApiView(page, uniqueName('Grid Clear Search'))
  await page.goto('/views?view=hierarchy')

  await page.getByTestId('views-search-input').fill(view.name)
  await expect(page.getByTestId('views-search-result').filter({ hasText: view.name })).toBeVisible()
  await page.getByLabel('Clear search').click()

  await expect(page.getByTestId('views-search-input')).toHaveValue('')
  await expect(page.getByTestId('views-search-results')).toHaveCount(0)
})

test('keyboard opens the active jump search result', async ({ page }) => {
  const view = await createApiView(page, uniqueName('Grid Keyboard Open'))
  await page.goto('/views?view=hierarchy')

  await page.getByTestId('views-search-input').fill(view.name)
  await page.keyboard.press('Enter')

  await expect(page).toHaveURL(new RegExp(`/views/${view.id}$`))
  await expect(page.getByTestId('vieweditor-canvas')).toBeVisible()
})

test('view details panel updates name and level label', async ({ page }) => {
  const view = await createApiView(page, uniqueName('Grid Details'))
  const nextName = uniqueName('Grid Details Renamed')
  await page.goto('/views?view=hierarchy')

  await openNodeMenu(page, view)
  const detailsItem = page.getByTestId('views-grid-node-details')
  await expect(detailsItem).toBeVisible()
  await detailsItem.click()

  const panel = page.getByTestId('view-panel')
  await expect(panel).toBeVisible()
  const nameInput = panel.getByTestId('view-panel-name-input')
  const labelInput = panel.getByTestId('view-panel-label-input')
  await expect(nameInput).toHaveValue(view.name)
  await expect(labelInput).toBeEditable()
  await nameInput.fill(nextName)
  await expect(nameInput).toHaveValue(nextName)
  await labelInput.fill('System Context')
  await expect(labelInput).toHaveValue('System Context')
  await labelInput.evaluate((input) => (input as HTMLInputElement).blur())

  const save = panel.getByTestId('view-panel-save')
  await expect(save).toBeEnabled()
  const updateResponse = waitForUpdateView(page)
  await save.click()
  expect((await updateResponse).ok()).toBeTruthy()

  await expect.poll(async () => {
    const updated = await getView(page, view.id)
    return { name: updated.name, label: updated.levelLabel ?? updated.level_label }
  }).toEqual({ name: nextName, label: 'System Context' })
})

test('view rename action commits from the grid card', async ({ page }) => {
  const view = await createApiView(page, uniqueName('Grid Rename'))
  const nextName = uniqueName('Grid Rename Next')
  await page.goto('/views?view=hierarchy')

  const node = await openNodeMenu(page, view)
  const renameItem = page.getByTestId('views-grid-node-rename')
  await expect(renameItem).toBeVisible()
  await renameItem.click()

  const renameInput = node.getByTestId('views-grid-node-rename-input')
  await expect(renameInput).toBeVisible()
  await expect(renameInput).toBeFocused()
  await expect(renameInput).toHaveValue(view.name)
  await renameInput.fill(nextName)
  await expect(renameInput).toHaveValue(nextName)

  const updateResponse = waitForUpdateView(page)
  await renameInput.press('Enter')
  expect((await updateResponse).ok()).toBeTruthy()

  await expect.poll(async () => (await getView(page, view.id)).name).toBe(nextName)
})

test('delete action can be canceled without removing the view', async ({ page }) => {
  const view = await createApiView(page, uniqueName('Grid Delete Cancel'))
  await page.goto('/views?view=hierarchy')

  await openNodeMenu(page, view)
  await page.getByTestId('views-grid-node-delete').click()
  await expect(page.getByTestId('confirm-dialog')).toBeVisible()
  await page.getByTestId('confirm-dialog-cancel').click()

  await expect.poll(async () => (await getView(page, view.id)).name).toBe(view.name)
})

test('delete action removes a view after confirmation', async ({ page }) => {
  const view = await createApiView(page, uniqueName('Grid Delete Confirm'))
  await page.goto('/views?view=hierarchy')

  await openNodeMenu(page, view)
  await page.getByTestId('views-grid-node-delete').click()
  await page.getByTestId('confirm-dialog-confirm').click()

  await expect.poll(async () => {
    const response = await page.request.post('/api/diag.v1.WorkspaceService/GetView', { data: { viewId: view.id } })
    return response.status()
  }).not.toBe(200)
})

test('view mode toggle keeps state in the URL', async ({ page }) => {
  await createApiView(page, uniqueName('Grid Mode Toggle'))
  await page.goto('/views?view=hierarchy')

  await page.getByRole('button', { name: 'Explore view' }).click()
  await expect(page).toHaveURL(/view=explore/)

  await page.getByRole('button', { name: 'Hierarchy view' }).click()
  await expect(page).toHaveURL(/view=hierarchy/)
})

test('search result click opens the selected diagram', async ({ page }) => {
  const view = await createApiView(page, uniqueName('Grid Search Click'))
  await page.goto('/views?view=hierarchy')

  await page.getByTestId('views-search-input').fill(view.name)
  await page.getByTestId('views-search-result').filter({ hasText: view.name }).click()

  await expect(page).toHaveURL(new RegExp(`/views/${view.id}$`))
})

test('deleted views disappear from filtered search results', async ({ page }) => {
  const view = await createApiView(page, uniqueName('Grid Gone'))
  await deleteView(page, view.id)

  await page.goto('/views?view=hierarchy')
  await page.getByTestId('views-search-input').fill(view.name)

  await expect(page.getByTestId('views-search-result').filter({ hasText: view.name })).toHaveCount(0)
})
