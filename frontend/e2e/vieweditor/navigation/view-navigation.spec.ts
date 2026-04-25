import { expect, test } from '@playwright/test'
import {
  createAndLoadDiagramWithNodes,
  createApiView,
  gotoView,
  nodeByName,
  prepareStorage,
} from '../../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await prepareStorage(page)
})

test('navigates to a child view from the node zoom control and back from the child node', async ({ page }) => {
  const { diagram, elements } = await createAndLoadDiagramWithNodes(page, 1, 'Keyboard Nav')
  const child = await createApiView(page, `${elements[0].name} Child`, elements[0].id)
  await page.reload()

  await nodeByName(page, elements[0].name).getByTestId('vieweditor-node-zoom-in').click()
  await expect(page).toHaveURL(new RegExp(`/views/${child.id}$`))

  await page.getByTestId('vieweditor-canvas').click()
  await page.keyboard.press('w')
  await expect(page).toHaveURL(new RegExp(`/views/${diagram.id}$`))
})

test('searches and opens a view from the explorer tree', async ({ page }) => {
  const { diagram } = await createAndLoadDiagramWithNodes(page, 0, 'Explorer Source')
  const target = await createApiView(page, 'Explorer Target View')
  await gotoView(page, diagram.id)

  await page.getByTestId('view-explorer-search').fill(target.name)
  await page.getByTestId('view-explorer-tree-item').filter({ hasText: target.name }).click()

  await expect(page).toHaveURL(new RegExp(`/views/${target.id}$`))
})

test('Shift+S creates a child view from the first element and navigates to it', async ({ page }) => {
  const { elements } = await createAndLoadDiagramWithNodes(page, 1, 'Create Child')
  await page.keyboard.press('Shift+S')

  await expect(page).toHaveURL(/\/views\/\d+$/)
  await expect(page.getByTestId('vieweditor-canvas')).toBeVisible()
  await expect(page.getByText(elements[0].name).first()).toBeVisible()
  await expect(nodeByName(page, elements[0].name)).toHaveCount(0)
})
