import type { Page } from '@playwright/test'
import { expect, test } from '../fixtures'
import { createApiView, createPlacedElement, uniqueName } from '../helpers/vieweditor'

async function waitForExploreCanvas(page: Page) {
  await expect(page.locator('canvas')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Fit View', exact: true })).toBeVisible()
}

function trackWorkspaceRequests(page: Page) {
  const requests: { method: string; body: Record<string, unknown> }[] = []
  page.on('request', (request) => {
    const method = request.url().split('/').at(-1) ?? ''
    if (method === 'GetWorkspace' || method === 'ListViews') {
      requests.push({ method, body: request.postDataJSON() })
    }
  })
  return requests
}

test('loads one workspace for the page, search, and explore canvas', async ({ page }) => {
  const view = await createApiView(page, uniqueName('Shared Workspace'))
  const element = await createPlacedElement(page, view.id, { name: uniqueName('Shared Element'), kind: 'service' })
  const requests = trackWorkspaceRequests(page)

  await page.goto('/views?view=explore')
  await waitForExploreCanvas(page)
  expect(requests).toEqual([{ method: 'GetWorkspace', body: { includeContent: true } }])

  await page.getByTestId('views-search-input').fill(view.name)
  await expect(page.getByTestId('views-search-result').filter({ hasText: view.name })).toBeVisible()
  await page.getByTestId('views-search-input').fill(element.name)
  await page.getByTestId('views-search-result').filter({ hasText: element.name }).click()
  await expect(page).toHaveURL(new RegExp(`view=explore&focus=${view.id}&element=${element.id}`))
  expect(requests).toHaveLength(1)
})

test('loads fresh content when returning from hierarchy to explore', async ({ page }) => {
  const view = await createApiView(page, uniqueName('Fresh Workspace'))
  await createPlacedElement(page, view.id, { name: uniqueName('Initial Element'), kind: 'service' })
  const requests = trackWorkspaceRequests(page)

  await page.goto('/views?view=explore')
  await waitForExploreCanvas(page)
  await page.getByRole('button', { name: 'Hierarchy view', exact: true }).click()
  await expect(page.locator('.react-flow')).toBeVisible()
  const element = await createPlacedElement(page, view.id, { name: uniqueName('Fresh Element'), kind: 'service' })
  await page.getByRole('button', { name: 'Explore view', exact: true }).click()
  await waitForExploreCanvas(page)

  await page.getByTestId('views-search-input').fill(element.name)
  await expect(page.getByTestId('views-search-result').filter({ hasText: element.name })).toBeVisible()
  expect(requests.filter(({ method, body }) => method === 'GetWorkspace' && !body.hasView)).toHaveLength(2)
  expect(requests.some(({ method }) => method === 'ListViews')).toBe(false)
})

test('keeps hierarchy search lazy without reloading the hierarchy grid', async ({ page }) => {
  const view = await createApiView(page, uniqueName('Hierarchy Workspace'))
  const element = await createPlacedElement(page, view.id, { name: uniqueName('Hierarchy Element'), kind: 'service' })
  const requests = trackWorkspaceRequests(page)
  const gridLoaded = page.waitForResponse((response) =>
    response.url().endsWith('/GetWorkspace') && response.request().postDataJSON().hasView === true,
  )

  await page.goto('/views?view=hierarchy')
  await gridLoaded
  await expect(page.locator('.react-flow')).toBeVisible()
  expect(requests).toHaveLength(2)

  await page.getByTestId('views-search-input').fill(element.name.slice(0, 2))
  expect(requests).toHaveLength(2)
  await page.getByTestId('views-search-input').fill(element.name)
  await expect(page.getByTestId('views-search-result').filter({ hasText: element.name })).toBeVisible()
  expect(requests).toHaveLength(3)
  expect(requests[2]).toEqual({ method: 'GetWorkspace', body: { includeContent: true } })
})

test('keeps explicit explore and default hierarchy behavior for empty workspaces', async ({ page }) => {
  await page.route('**/api/diag.v1.WorkspaceService/GetWorkspace', (route) => route.fulfill({
    json: { views: [], content: {}, navigations: [] },
  }))
  const requests = trackWorkspaceRequests(page)

  await page.goto('/views?view=explore')
  await expect(page.getByText('No diagrams to explore yet', { exact: true })).toBeVisible()
  await expect(page).toHaveURL(/view=explore$/)
  expect(requests).toHaveLength(1)

  await page.goto('/views')
  await expect(page).toHaveURL(/view=hierarchy$/)
  await expect(page.locator('.react-flow')).toBeVisible()
  expect(requests).toHaveLength(2)
  expect(requests.every(({ method, body }) => method === 'GetWorkspace' && body.includeContent === true)).toBe(true)
})
