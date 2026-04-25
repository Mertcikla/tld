import { expect, test } from '@playwright/test'
import {
  createApiView,
  prepareStorage,
  uniqueName,
} from '../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await prepareStorage(page)
})

test('creates a diagram from the views grid and opens it', async ({ page }) => {
  await page.goto('/views?view=hierarchy')
  const name = uniqueName('Views Grid Create')

  await page.getByTestId('views-new-diagram-button').click()
  await page.getByTestId('views-new-diagram-name-input').fill(name)
  await page.getByTestId('views-create-diagram-submit').click()

  await expect(page).toHaveURL(/\/views\/\d+$/)
  await expect(page.getByTestId('vieweditor-canvas')).toBeVisible()
})

test('searches and opens an existing diagram from the views grid', async ({ page }) => {
  const view = await createApiView(page, uniqueName('Views Grid Search'))
  await page.goto('/views?view=hierarchy')

  await page.getByTestId('views-search-input').fill(view.name)
  await page.keyboard.press('Enter')

  await expect(page).toHaveURL(new RegExp(`/views/${view.id}$`))
})
