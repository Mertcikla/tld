import { beforeEach, describe, expect, it, vi } from 'vitest'

function installWindow(overrides: Record<string, unknown> = {}) {
  const open = vi.fn()
  const body = {
    appendChild: vi.fn(),
    removeChild: vi.fn(),
  }
  Object.defineProperty(globalThis, 'window', {
    configurable: true,
    value: {
      location: { href: 'http://localhost:5173/views/1' },
      open,
      ...overrides,
    },
  })
  Object.defineProperty(globalThis, 'document', {
    configurable: true,
    value: {
      createElement: vi.fn(() => ({ click: vi.fn(), href: '', download: '' })),
      body,
    },
  })
  Object.defineProperty(globalThis.URL, 'createObjectURL', {
    configurable: true,
    value: vi.fn(() => 'blob:test'),
  })
  Object.defineProperty(globalThis.URL, 'revokeObjectURL', {
    configurable: true,
    value: vi.fn(),
  })
  return { open, body }
}

describe('desktop helpers', () => {
  beforeEach(() => {
    vi.resetModules()
    vi.restoreAllMocks()
  })

  it('opens external urls through window.open in browser mode', async () => {
    const { open } = installWindow()
    const { openExternalUrl } = await import('./desktop')

    openExternalUrl('https://example.com/docs')

    expect(open).toHaveBeenCalledWith('https://example.com/docs', '_blank', 'noopener,noreferrer')
  })

  it('opens external urls through Wails runtime in desktop mode', async () => {
    const browserOpen = vi.fn()
    installWindow({ __TLD_APP__: true, runtime: { BrowserOpenURL: browserOpen } })
    const { openExternalUrl } = await import('./desktop')

    openExternalUrl('https://example.com/docs')

    expect(browserOpen).toHaveBeenCalledWith('https://example.com/docs')
  })

  it('saves blobs through the desktop bridge in Wails mode', async () => {
    const saveFile = vi.fn().mockResolvedValue({ path: '/tmp/diagram.md', canceled: false })
    installWindow({ __TLD_APP__: true, go: { main: { DesktopBridge: { SaveFile: saveFile } } } })
    const { saveBlobAs } = await import('./desktop')

    const result = await saveBlobAs(new Blob(['```mermaid\nflowchart LR\n```\n']), 'diagram.md', [
      { displayName: 'Markdown Files (*.md)', pattern: '*.md' },
    ])

    expect(result).toEqual({ path: '/tmp/diagram.md', canceled: false })
    expect(saveFile).toHaveBeenCalledWith(
      'diagram.md',
      [{ displayName: 'Markdown Files (*.md)', pattern: '*.md' }],
      'YGBgbWVybWFpZApmbG93Y2hhcnQgTFIKYGBgCg==',
    )
  })

  it('returns canceled open results from the desktop bridge', async () => {
    const openText = vi.fn().mockResolvedValue({ path: '', content: '', canceled: true })
    installWindow({ __TLD_APP__: true, go: { main: { DesktopBridge: { OpenTextFile: openText } } } })
    const { openTextFile } = await import('./desktop')

    await expect(openTextFile()).resolves.toEqual({ path: '', content: '', canceled: true })
  })

  it('checks desktop updates through the bridge in Wails mode', async () => {
    const updateStatus = {
      checked: true,
      current: '2.2.4',
      latest: 'v2.2.5',
      updateAvailable: true,
      releaseUrl: 'https://github.com/Mertcikla/tld/releases/tag/v2.2.5',
      assetName: 'tld-desktop-macos-arm64.zip',
      cached: false,
      supported: true,
      canInstall: true,
      installStarted: false,
      restartRequired: false,
    }
    const checkForUpdate = vi.fn().mockResolvedValue(updateStatus)
    installWindow({ __TLD_APP__: true, go: { main: { DesktopBridge: { CheckForUpdate: checkForUpdate } } } })
    const { checkForDesktopUpdate } = await import('./desktop')

    await expect(checkForDesktopUpdate()).resolves.toEqual(updateStatus)
    expect(checkForUpdate).toHaveBeenCalled()
  })

  it('rejects desktop update checks outside Wails mode', async () => {
    installWindow()
    const { checkForDesktopUpdate } = await import('./desktop')

    await expect(checkForDesktopUpdate()).rejects.toThrow('desktop app')
  })

  it('rejects self updates in Mac App Store mode', async () => {
    const checkForUpdate = vi.fn()
    const installUpdate = vi.fn()
    installWindow({
      __TLD_APP__: true,
      __TLD_APP_STORE__: true,
      go: { main: { DesktopBridge: { CheckForUpdate: checkForUpdate, InstallUpdate: installUpdate } } },
    })
    const { checkForDesktopUpdate, installDesktopUpdate } = await import('./desktop')

    await expect(checkForDesktopUpdate()).rejects.toThrow('Mac App Store')
    await expect(installDesktopUpdate()).rejects.toThrow('Mac App Store')
    expect(checkForUpdate).not.toHaveBeenCalled()
    expect(installUpdate).not.toHaveBeenCalled()
  })
})
