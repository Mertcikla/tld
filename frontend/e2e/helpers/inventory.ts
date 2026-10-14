import { expect, type Page } from '@playwright/test'

export async function openInventory(page: Page, url = '/inventory') {
  await page.goto(url)
  // Loading replaces table headers and filter options.
  await expect(page.locator('[aria-busy="false"]')).toBeVisible()
}

export async function searchInventory(page: Page, query: string) {
  const search = page.getByTestId('inventory-search')
  // Commit each controlled-input change before starting the next search.
  await search.fill('')
  await expect(search).toHaveValue('')
  await expect.poll(() => new URL(page.url()).searchParams.get('q') ?? '').toBe('')
  await search.fill(query)
  await expect(search).toHaveValue(query)
  await expect.poll(() => new URL(page.url()).searchParams.get('q') ?? '').toBe(query)
}
