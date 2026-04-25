import { expect, test } from '@playwright/test'
import { prepareStorage } from '../helpers/vieweditor'

test.beforeEach(async ({ page }) => {
  await prepareStorage(page)
})

test('appearance settings persist theme choices in local storage', async ({ page }) => {
  await page.goto('/settings/appearance')

  await page.getByRole('button', { name: 'Teal accent color', exact: true }).click()
  await page.getByRole('button', { name: 'Coal background color', exact: true }).click()
  await page.getByRole('button', { name: 'Forest element color', exact: true }).click()

  await expect.poll(async () => page.evaluate(() => ({
    accent: localStorage.getItem('diag:accent-color'),
    background: localStorage.getItem('diag:background-color'),
    element: localStorage.getItem('diag:element-color'),
    cssAccent: getComputedStyle(document.documentElement).getPropertyValue('--accent').trim(),
  }))).toEqual({
    accent: '#4fd1c5',
    background: '#111111',
    element: '#22543d',
    cssAccent: '#4fd1c5',
  })

  await page.reload()
  await expect(page.getByLabel('Teal accent color (active)')).toBeVisible()
})
