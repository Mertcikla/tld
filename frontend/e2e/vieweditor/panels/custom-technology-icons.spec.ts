import { Buffer } from 'node:buffer'
import type { Locator, Page } from '@playwright/test'
import { expect, test } from '../../fixtures'
import {
  createAndLoadDiagramWithNodes,
  openElementPanel,
  uniqueName,
} from '../../helpers/vieweditor'

const customIconSvg = `
<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32" viewBox="0 0 32 32">
  <rect width="32" height="32" rx="6" fill="#38bdf8"/>
  <circle cx="16" cy="16" r="8" fill="#0f172a"/>
</svg>
`.trim()

test('custom technology upload creates an icon asset that renders after reload', async ({ page }) => {
  const technologyName = uniqueName('E2E Custom Icon')
  const shortName = 'E2E'
  const { elements } = await createAndLoadDiagramWithNodes(page, 1, 'Custom Icon')

  await openElementPanel(page, elements[0].name)
  await page.getByTestId('element-panel-technology-input').fill(technologyName)
  await page.getByTestId('element-panel-custom-technology-create').click()
  await expect(page.getByTestId('custom-technology-file')).toBeAttached()

  await page.getByTestId('custom-technology-file').setInputFiles({
    name: `${technologyName.toLowerCase().replace(/[^a-z0-9]+/g, '-')}.svg`,
    mimeType: 'image/svg+xml',
    buffer: Buffer.from(customIconSvg),
  })
  await expect(page.getByTestId('custom-technology-preview-icon')).toBeVisible()
  await page.getByTestId('custom-technology-options-toggle').click()
  await page.getByTestId('custom-technology-short-name').fill(shortName)
  await page.getByTestId('custom-technology-aliases').fill('e2e-custom-alias')

  await page.getByTestId('custom-technology-save').click()

  const chip = page.getByTestId('element-panel-technology-chip').filter({ hasText: technologyName })
  await expect(chip).toBeVisible()
  const chipIcon = chip.locator('img').first()
  await expectImageLoaded(chipIcon)

  const iconPath = await iconPathFromImage(page, chipIcon)
  expect(iconPath).toMatch(/^\/icons\/.+\.svg$/)
  await expectIconAsset(page, iconPath, 'image/svg+xml')

  const panel = page.getByTestId('element-panel').filter({ visible: true }).last()
  await panel.getByTestId('panel-close').click()
  await expect(panel).toBeHidden()

  await page.reload()
  await openElementPanel(page, elements[0].name)

  const reloadedChip = page.getByTestId('element-panel-technology-chip').filter({ hasText: technologyName })
  await expect(reloadedChip).toBeVisible()
  const reloadedIcon = reloadedChip.locator('img').first()
  await expectImageLoaded(reloadedIcon)
  await expect(iconPathFromImage(page, reloadedIcon)).resolves.toBe(iconPath)
  await expectIconAsset(page, iconPath, 'image/svg+xml')
})

test('missing icon asset requests return 404 instead of the app shell', async ({ page }) => {
  const response = await page.request.get('/icons/__tld_e2e_missing_custom_icon__.svg')

  expect(response.status()).toBe(404)
  expect(await response.text()).not.toContain('<html')
})

async function expectImageLoaded(locator: Locator) {
  await expect(locator).toBeVisible()
  await expect.poll(async () => locator.evaluate((element) => {
    const image = element as HTMLImageElement
    return image.complete && image.naturalWidth > 0 && image.naturalHeight > 0
  })).toBe(true)
}

async function iconPathFromImage(page: Page, locator: Locator) {
  const src = await locator.getAttribute('src')
  expect(src).toBeTruthy()
  return new URL(src!, page.url()).pathname
}

async function expectIconAsset(page: Page, iconPath: string, contentType: string) {
  const response = await page.request.get(iconPath)

  expect(response.status()).toBe(200)
  expect(response.headers()['content-type']).toContain(contentType)
  expect(await response.text()).toContain('<svg')
}
