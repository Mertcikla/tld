import { expect, test } from '../fixtures'
import {
  createApiView,
  createDiagram,
  uniqueName,
} from '../helpers/vieweditor'

async function clickVisibleNav(page: import('@playwright/test').Page, desktopTestId: string, mobileTestId: string) {
  const desktop = page.getByTestId(desktopTestId)
  if (await desktop.isVisible().catch(() => false)) {
    await desktop.click()
    return
  }
  await page.getByTestId(mobileTestId).click()
}

async function openAppearance(page: import('@playwright/test').Page) {
  const desktop = page.getByTestId('topnav-appearance')
  if (await desktop.isVisible().catch(() => false)) {
    await desktop.click()
    return
  }
  await page.getByTestId('mobile-topnav-appearance').click()
}

test('home redirects to the first available root diagram', async ({ page }) => {
  await createApiView(page, uniqueName('Home Redirect Root'))

  await page.goto('/')

  await expect(page).toHaveURL(/\/views\/\d+$/)
  await expect(page.getByTestId('vieweditor-canvas')).toBeVisible()
})

test('top navigation opens core pages and marks route changes', async ({ page }) => {
  await createDiagram(page, uniqueName('Top Nav Diagram'))

  await clickVisibleNav(page, 'topnav-inventory', 'mobile-topnav-inventory')
  await expect(page).toHaveURL(/\/inventory$/)
  await expect(page.getByTestId('inventory-page')).toBeVisible()

  await clickVisibleNav(page, 'topnav-diagrams', 'mobile-topnav-diagrams')
  await expect(page).toHaveURL(/\/views(\?.*)?$/)
  await expect(page.getByRole('button', { name: 'Hierarchy view' })).toBeVisible()

  await clickVisibleNav(page, 'topnav-editor', 'mobile-topnav-editor')
  await expect(page).toHaveURL(/\/views\/\d+$/)
  await expect(page.getByTestId('vieweditor-canvas')).toBeVisible()
})

test('appearance popover applies settings from the top bar', async ({ page }) => {
  await createDiagram(page, uniqueName('Appearance Popover'))

  await openAppearance(page)
  await page.getByRole('button', { name: 'Teal accent color', exact: true }).click()

  await expect.poll(async () => page.evaluate(() => localStorage.getItem('diag:accent-color'))).toBe('#4fd1c5')
})

test('settings api-key route falls back to appearance in local platform mode', async ({ page }) => {
  await page.goto('/settings/api-keys')

  await expect(page).toHaveURL(/\/settings\/appearance$/)
  await expect(page.getByRole('button', { name: /accent color/ }).first()).toBeVisible()
})

test('mobile bottom navigation reaches app pages', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 760 })
  await createDiagram(page, uniqueName('Mobile Nav'))

  await page.getByTestId('mobile-topnav-inventory').dispatchEvent('click')
  await expect(page).toHaveURL(/\/inventory$/)

  await page.getByTestId('mobile-topnav-diagrams').dispatchEvent('click')
  await expect(page).toHaveURL(/\/views(\?.*)?$/)
})
