import { spawn, type ChildProcess } from 'node:child_process'
import { createWriteStream, mkdirSync, readFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import {
  expect,
  test as base,
  type Browser,
  type BrowserContext,
  type ConsoleMessage,
  type Page,
  type TestInfo,
  type WorkerInfo,
} from '@playwright/test'
import { prepareE2EPage } from './helpers/vieweditor'

type E2EServer = {
  baseURL: string
  dataDir?: string
  logPath?: string
}

type TestFixtures = {
  e2eErrorMonitor: void
}

type WorkerFixtures = {
  e2eServer: E2EServer
}

type CapturedE2EError = {
  kind: 'pageerror' | 'console'
  message: string
  location?: string
  stack?: string
}

const serverReadyTimeout = 30_000
const serverShutdownTimeout = 10_000
const workerPortOffset = 100

const e2eDir = dirname(fileURLToPath(import.meta.url))
const frontendDir = resolve(e2eDir, '..')
const repoRoot = resolve(frontendDir, '..')

export const test = base.extend<TestFixtures, WorkerFixtures>({
  e2eServer: [
    async ({}, use, workerInfo) => {
      const externalBaseURL = process.env.TLD_E2E_BASE_URL?.trim()
      if (externalBaseURL) {
        await waitForReady(externalBaseURL)
        await use({ baseURL: externalBaseURL })
        return
      }

      const basePort = Number(process.env.TLD_E2E_PORT ?? 8060)
      if (!Number.isInteger(basePort) || basePort < 1 || basePort > 65535) {
        throw new Error(`Invalid TLD_E2E_PORT: ${process.env.TLD_E2E_PORT}`)
      }

      const port = basePort + workerInfo.workerIndex * workerPortOffset
      if (port > 65535) {
        throw new Error(`Worker ${workerInfo.workerIndex} derived invalid E2E port ${port} from base port ${basePort}`)
      }

      const baseURL = `http://127.0.0.1:${port}`
      const dataDir = workerDataDir(workerInfo)
      const logPath = join(dataDir, 'server.log')
      mkdirSync(dataDir, { recursive: true })

      const server = spawnServer(port, dataDir, logPath)
      try {
        await waitForReady(baseURL, () => serverExitError(server, logPath))
        await use({ baseURL, dataDir, logPath })
      } finally {
        await stopServer(server)
      }
    },
    { scope: 'worker' },
  ],

  baseURL: async ({ e2eServer }, use) => {
    await use(e2eServer.baseURL)
  },

  page: async ({ page, baseURL }, use) => {
    await prepareE2EPage(page)
    await waitForReady(requireBaseURL(baseURL))
    await use(page)
  },

  e2eErrorMonitor: [
    async ({ browser, page }, use, testInfo) => {
      const monitor = createErrorMonitor(testInfo)
      monitor.monitorPage(page)

      for (const context of browser.contexts()) {
        monitor.monitorContext(context)
      }

      const browserWithPatchedContext = browser as Browser & { newContext: Browser['newContext'] }
      const originalNewContext = browserWithPatchedContext.newContext.bind(browser)
      browserWithPatchedContext.newContext = (async (...args: Parameters<Browser['newContext']>) => {
        const context = await originalNewContext(...args)
        monitor.monitorContext(context)
        return context
      }) as Browser['newContext']

      try {
        await use()
      } finally {
        browserWithPatchedContext.newContext = originalNewContext as Browser['newContext']
      }

      monitor.assertNoErrors()
    },
    { auto: true },
  ],
})

export { expect }

function workerDataDir(workerInfo: WorkerInfo) {
  const root = process.env.TLD_E2E_DATA_DIR
    ?? join(tmpdir(), `tld-playwright-${process.env.GITHUB_RUN_ID ?? 'local'}`)
  const project = sanitizePathSegment(workerInfo.project.name || 'default')
  return join(root, project, `worker-${workerInfo.workerIndex}`)
}

function sanitizePathSegment(value: string) {
  return value.replace(/[^a-zA-Z0-9._-]+/g, '-').replace(/^-+|-+$/g, '') || 'default'
}

function spawnServer(port: number, dataDir: string, logPath: string) {
  const binary = process.env.TLD_E2E_BINARY || 'tld'
  const configDir = join(dataDir, 'config')
  mkdirSync(configDir, { recursive: true })
  const log = createWriteStream(logPath, { flags: 'a' })
  const child = spawn(binary, [
    'serve',
    '--foreground',
    '--host',
    '127.0.0.1',
    '--port',
    String(port),
    '--data-dir',
    dataDir,
  ], {
    cwd: repoRoot,
    env: {
      ...process.env,
      TLD_CONFIG_DIR: configDir,
      TLD_SKIP_STARTUP_UPDATE: '1',
    },
    stdio: ['ignore', 'pipe', 'pipe'],
  })

  child.stdout?.pipe(log)
  child.stderr?.pipe(log)
  child.once('close', () => log.end())
  return child
}

async function stopServer(child: ChildProcess) {
  if (child.exitCode !== null || child.signalCode !== null) return

  child.kill('SIGTERM')
  const exited = waitForExit(child)
  let timeoutId: ReturnType<typeof setTimeout> | null = null
  const timeout = new Promise<void>((resolveTimeout) => {
    timeoutId = setTimeout(() => resolveTimeout(), serverShutdownTimeout)
  }).then(async () => {
    if (child.exitCode === null && child.signalCode === null) child.kill('SIGKILL')
    await exited
  })

  try {
    await Promise.race([exited, timeout])
  } finally {
    if (timeoutId !== null) clearTimeout(timeoutId)
  }
}

function waitForExit(child: ChildProcess) {
  if (child.exitCode !== null || child.signalCode !== null) return Promise.resolve()
  return new Promise<void>((resolveExit) => {
    child.once('exit', () => resolveExit())
  })
}

async function waitForReady(baseURL: string, getProcessError?: () => Error | null) {
  const deadline = Date.now() + serverReadyTimeout
  let lastError: unknown
  const readyURL = new URL('/api/ready', baseURL)

  while (Date.now() < deadline) {
    const processError = getProcessError?.()
    if (processError) throw processError

    try {
      const response = await fetch(readyURL)
      if (response.ok) return
      lastError = new Error(`ready check returned ${response.status}`)
    } catch (error) {
      lastError = error
    }

    await delay(250)
  }

  throw new Error(`E2E server at ${baseURL} did not become ready: ${formatUnknownError(lastError)}`)
}

function serverExitError(child: ChildProcess, logPath: string) {
  if (child.exitCode === null && child.signalCode === null) return null

  const status = child.exitCode !== null
    ? `exit code ${child.exitCode}`
    : `signal ${child.signalCode}`
  return new Error(`E2E server exited before readiness with ${status}. Log: ${logPath}\n${tailFile(logPath)}`)
}

function requireBaseURL(baseURL: string | undefined) {
  if (!baseURL) throw new Error('Expected E2E baseURL to be configured by the e2eServer fixture')
  return baseURL
}

function createErrorMonitor(testInfo: TestInfo) {
  const errors: CapturedE2EError[] = []
  const monitoredPages = new WeakSet<Page>()
  const monitoredContexts = new WeakSet<BrowserContext>()

  const monitorPage = (page: Page) => {
    if (monitoredPages.has(page)) return
    monitoredPages.add(page)

    page.on('pageerror', (error) => {
      if (isAllowedPageError(error)) return

      errors.push({
        kind: 'pageerror',
        message: error.message,
        stack: error.stack,
      })
    })

    page.on('console', (message) => {
      if (message.type() !== 'error') return
      if (isAllowedConsoleError(message)) return

      const location = message.location()
      errors.push({
        kind: 'console',
        message: message.text(),
        location: formatConsoleLocation(location),
      })
    })
  }

  const monitorContext = (context: BrowserContext) => {
    if (monitoredContexts.has(context)) return
    monitoredContexts.add(context)

    for (const existingPage of context.pages()) {
      monitorPage(existingPage)
    }
    context.on('page', monitorPage)
  }

  return {
    monitorContext,
    monitorPage,
    assertNoErrors() {
      if (errors.length === 0) return

      const details = errors.map((error, index) => formatCapturedError(index + 1, error)).join('\n\n')
      testInfo.annotations.push({
        type: 'e2e-error-monitor',
        description: `${errors.length} unexpected browser error(s)`,
      })
      throw new Error(`Unexpected browser errors detected:\n\n${details}`)
    },
  }
}

function isAllowedConsoleError(message: ConsoleMessage) {
  const text = message.text()
  const location = message.location()
  const target = `${text} ${location.url ?? ''}`

  if (/ResizeObserver loop (?:limit exceeded|completed with undelivered notifications)/i.test(text)) {
    return true
  }

  if (/DIAGRAM EDITOR LOAD ERROR: Error: \[unknown\] Load failed/i.test(text)) {
    return true
  }

  if (/^DIAGRAM EDITOR LOAD ERROR: Error$/i.test(text)) {
    return true
  }

  if (/Failed to load thumbnail: TypeError: Load failed/i.test(text)) {
    return true
  }

  if (/Failed to fetch elements: Error: \[unknown\] (?:Load failed|Failed to fetch)/i.test(text)) {
    return true
  }

  const isResourceLoadError = /Failed to load resource/i.test(text) || /net::ERR_/i.test(text)
  if (!isResourceLoadError) return false

  return /\/favicon\.(?:ico|svg)\b/i.test(target)
    || /\/icons\/[^ ]+\.(?:svg|png|webp)\b/i.test(target)
    || /https:\/\/fonts\.(?:googleapis|gstatic)\.com\b/i.test(target)
}

function isAllowedPageError(error: Error) {
  const text = `${error.message}\n${error.stack ?? ''}`

  if (/ResizeObserver loop (?:limit exceeded|completed with undelivered notifications)/i.test(text)) {
    return true
  }

  if (/Fetch API cannot load http:\/\/127\.0\.0\.1:\d+\/(?:api\/diag\.v1\.WorkspaceService\/Get(?:View|Workspace)|api\/views\/\d+\/(?:thumbnail\.svg|populate-query)) due to access control checks/i.test(text)) {
    return true
  }

  return /(?:Unhandled Promise Rejection: )?Error: \[unknown\] Load failed/i.test(text)
}

function formatCapturedError(index: number, error: CapturedE2EError) {
  const parts = [`${index}. ${error.kind}: ${error.message}`]
  if (error.location) parts.push(`   at ${error.location}`)
  if (error.stack) parts.push(error.stack)
  return parts.join('\n')
}

function formatConsoleLocation(location: ReturnType<ConsoleMessage['location']>) {
  if (!location.url) return undefined
  const line = location.lineNumber ? `:${location.lineNumber}` : ''
  const column = location.columnNumber ? `:${location.columnNumber}` : ''
  return `${location.url}${line}${column}`
}

function tailFile(path: string) {
  try {
    return readFileSync(path, 'utf8').split('\n').slice(-40).join('\n')
  } catch {
    return ''
  }
}

function formatUnknownError(error: unknown) {
  if (error instanceof Error) return error.message
  return String(error)
}

function delay(ms: number) {
  return new Promise<void>((resolveDelay) => setTimeout(resolveDelay, ms))
}
