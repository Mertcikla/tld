import { mkdtempSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { execFileSync } from 'node:child_process'
import { createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { RepositoryService } from '../../src/gen/codeindex/v1/codeindex_pb'
import { test, expect } from '../fixtures'
import { createApiView, uniqueName } from '../helpers/vieweditor'

test('plan handoff, editor drawer, and recovery work without Watch', async ({ page, baseURL }, testInfo) => {
  const root = mkdtempSync(join(tmpdir(), 'tld-plan-e2e-'))
  const git = (...args: string[]) => execFileSync('git', ['-C', root, ...args], { stdio: 'pipe' })
  try {
    git('init'); git('config', 'user.name', 'Plan Test'); git('config', 'user.email', 'plan@example.test')
    writeFileSync(join(root, 'README.md'), '# Plan workflow fixture\n')
    git('add', 'README.md'); git('commit', '-m', 'Initial commit')
    const client = createClient(RepositoryService, createConnectTransport({ baseUrl: `${baseURL}/api` }))
    let repositoryId = ''
    for await (const event of client.addRepository({ path: root })) {
      if (event.event.case === 'repository') repositoryId = event.event.value.id
    }
    expect(repositoryId).not.toBe('')
    const view = await createApiView(page, uniqueName('Plan architecture'))
    const rpc = async (method: string, body: unknown) => {
      const response = await page.request.post(`/api/diag.v1.WorkspaceService/${method}`, { data: body })
      expect(response.ok(), await response.text()).toBe(true)
      return response.json()
    }
    const elementName = uniqueName('Plan worker')
    await rpc('CreateElement', { name: elementName })
    await page.route('**/ready', route => route.fulfill({ json: { ok: true, capabilities: { watch: false, editor: true, repositories: true } } }))
    await page.goto(`/repositories?repo=${repositoryId}&mode=plans`)
    await page.getByRole('button', { name: 'New plan', exact: true }).click()
    await expect(page.getByLabel('Plan objective')).toBeVisible()
    await page.getByLabel('Plan objective').fill('Implement bounded retries')
    await page.getByLabel('Search plan scope').fill(elementName)
    await page.getByRole('button', { name: `Add ${elementName}`, exact: true }).click()
    await page.getByRole('button', { name: elementName, exact: true }).click()
    await page.getByTestId('element-panel-name-input').fill(elementName + ' updated')
    // Include a slow property save in the handoff barrier.
    await page.route('**/api/diag.v1.PlanService/AppendOperations', async route => {
      await new Promise(resolve => setTimeout(resolve, 250))
      await route.continue()
    })
    // Starting immediately after typing must flush property edits and metadata.
    await page.getByRole('button', { name: 'Start implementation', exact: true }).click()
    await expect(page.getByTestId('agent-workspace-review')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Copy prompt', exact: true })).toBeEnabled()
    await expect(page.getByTestId('repositories-live-tab')).toHaveCount(0)
    const planId = new URL(page.url()).searchParams.get('plan')!
    // A global recovery link must work from an already-mounted settings screen.
    await page.goto(`/repositories?repo=${repositoryId}&mode=snapshots&page=settings`)
    await page.getByRole('button', { name: /^Implementing ·/ }).click()
    await page.getByRole('button', { name: 'Open plan and review', exact: true }).click()
    await expect(page.getByLabel('Plan objective')).toHaveValue('Implement bounded retries')
    // Refreshing must retain the lock and its independent recovery controls.
    await page.reload()
    await expect(page.getByTestId('agent-workspace-review')).toBeVisible()
    const prompt = await page.request.post('/api/diag.v1.PlanService/GetPrompt', { data: { planId } })
    const promptText = (await prompt.json()).text
    expect(promptText).toContain('Implement bounded retries')
    expect(promptText).toContain(elementName + ' updated')
    await page.goto(`/views/${view.id}?plan=${planId}&repo=${repositoryId}`)
    await page.getByRole('button', { name: /^Plan ·/ }).click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await expect(page.getByLabel('Plan objective')).toHaveValue('Implement bounded retries')
    // Wait for the drawer's slide animation before checking responsive layout.
    await expect.poll(async () => {
      const box = await page.getByRole('dialog').boundingBox()
      return box ? box.x + box.width : Number.POSITIVE_INFINITY
    }).toBeLessThanOrEqual(page.viewportSize()!.width + 1)
    const dialog = await page.getByRole('dialog').boundingBox()
    expect(dialog!.width).toBeLessThanOrEqual(page.viewportSize()!.width + 1)
    if (page.viewportSize()!.width > 700) {
      // A persistent plan drawer must leave the architecture canvas interactive.
      await page.getByTestId('vieweditor-canvas').click({ position: { x: 100, y: 200 } })
      await expect(page.getByRole('dialog')).toBeVisible()
    }
    await page.screenshot({ path: testInfo.outputPath('plan-drawer.png') })
    await page.getByRole('button', { name: 'End without applying', exact: true }).click()
    await page.getByRole('button', { name: 'Discard pending updates and end', exact: true }).click()
    await expect(page.getByText('Implementation ended. Pending diagram updates discarded; source files are unchanged.')).toBeVisible()
    const status = await page.request.post('/api/diag.v1.PlanService/GetImplementation', { data: {} })
    expect((await status.json()).locked ?? false).toBe(false)
  } finally { rmSync(root, { recursive: true, force: true }) }
})
