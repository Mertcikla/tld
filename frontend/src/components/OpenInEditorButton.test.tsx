import React from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import OpenInEditorButton from './OpenInEditorButton'

vi.mock('../api/client', () => ({ api: { system: { capabilities: async () => ({ editor: false }) } } }))
vi.mock('../utils/sourceEditor', () => ({ useSourceEditor: () => ({ editor: 'zed' }) }))
vi.mock('../utils/toast', () => ({ toast: vi.fn() }))
vi.mock('@chakra-ui/icons', () => ({ ExternalLinkIcon: () => null }))
vi.mock('@chakra-ui/react', () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => children,
  Button: ({ children, as = 'button', ...props }: { children?: React.ReactNode; as?: string }) => React.createElement(as, props, children),
}))
let renderer: ReactTestRenderer

describe('OpenInEditorButton remote fallback', () => {
  afterEach(() => { act(() => { renderer.unmount() }) })

  it.each([
    ['src/api/', 'origin/develop', null, 'https://github.com/owner/repo/tree/develop/src/api/'],
    ['src/main.go', 'origin/release/v2', 3, 'https://github.com/owner/repo/blob/release%2Fv2/src/main.go#L3'],
    ['src/api/', 'develop', null, 'https://github.com/owner/repo/tree/develop/src/api/'],
    ['src/main.go', 'develop', 3, 'https://github.com/owner/repo/blob/develop/src/main.go#L3'],
    ['src/api/', null, null, 'https://github.com/owner/repo'],
  ])('opens the correct GitHub target for %s on %s', async (filePath, branch, line, expected) => {
    await act(async () => {
      renderer = create(<OpenInEditorButton repo="https://github.com/owner/repo" filePath={filePath} branch={branch} line={line} />)
    })
    expect(renderer.root.findByType('a').props.href).toBe(expected)
  })
})
