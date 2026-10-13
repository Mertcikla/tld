import { StrictMode } from 'react'
import { MemoryRouter, useLocation } from 'react-router-dom'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'

vi.mock('../config/runtime', () => ({ isWailsApp: true }))
vi.mock('../utils/toast', () => ({ toast: vi.fn() }))
import { useDesktopSceneLinks } from './useDesktopSceneLinks'
const workspaceMounted = vi.fn()

function Consumer() {
  const ready = useDesktopSceneLinks()
  const location = useLocation()
  if (ready && location.pathname === '/') workspaceMounted()
  return <output>{ready ? location.pathname + location.hash : 'loading'}</output>
}

afterEach(() => { vi.unstubAllGlobals() })

describe('desktop scene listener', () => {
  it('drains startup links and subsequent notifications in order', async () => {
    let notify = () => {}
    let pending: { route: string }[] = [{ route: '/compare#v1=YQ' }]
    const unsubscribe = vi.fn()
    const drain = vi.fn(async () => { const batch = pending; pending = []; return batch })
    vi.stubGlobal('window', { go: { main: { DesktopBridge: { DrainSceneLinks: drain } } }, runtime: { EventsOn: (_name: string, fn: () => void) => { notify = fn; return unsubscribe } } })
    let renderer!: ReactTestRenderer
    await act(async () => { renderer = create(<StrictMode><MemoryRouter><Consumer /></MemoryRouter></StrictMode>) })
    expect(renderer.root.findByType('output').children.join('')).toBe('/compare#v1=YQ')
    expect(workspaceMounted).not.toHaveBeenCalled()
    await act(async () => { pending = [{ route: '/compare#v1=Yg' }]; notify() })
    expect(renderer.root.findByType('output').children.join('')).toBe('/compare#v1=Yg')
    await act(async () => { pending = [{ route: 'https://untrusted.example/' }]; notify() })
    expect(renderer.root.findByType('output').children.join('')).toBe('/compare#v1=Yg')
    await act(async () => renderer.unmount())
    expect(unsubscribe).toHaveBeenCalled()
  })
})
