import { expect, test as base } from '@playwright/test'
import { prepareE2EPage } from './helpers/vieweditor'

export const test = base.extend({
  page: async ({ page }, use) => {
    await prepareE2EPage(page)
    await expect.poll(async () => {
      const response = await page.request.get('/api/ready')
      return response.ok()
    }).toBe(true)
    await use(page)
  },
})

export { expect }
