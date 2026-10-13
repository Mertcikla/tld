import { createElement } from 'react'
import { MemoryRouter } from 'react-router-dom'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { readFileSync } from 'node:fs'
import { describe, expect, it, vi } from 'vitest'

const captured = vi.hoisted(() => ({ embedded: undefined as unknown }))
vi.mock('@chakra-ui/react', () => Object.fromEntries(['Box', 'Button', 'Flex', 'Spinner', 'Text'].map((name) => [name, ({ children, ...props }: { children?: React.ReactNode }) => createElement('div', props, children)])))
vi.mock('../context/ThemeContext', () => ({ ThemeProvider: ({ children }: { children: React.ReactNode }) => children }))
vi.mock('../context/ConnectorStyleContext', () => ({ ConnectorStyleProvider: ({ children }: { children: React.ReactNode }) => children }))
vi.mock('../components/RepositoryChangeCanvas', () => ({ default: ({ embedded }: { embedded: unknown }) => { captured.embedded = embedded; return createElement('canvas') } }))
import ComparisonScenePage from './ComparisonScene'

const hash = '#' + readFileSync(new URL('../../../internal/mermaid/fixtures/scene-url.txt', import.meta.url), 'utf8').trim().split('#')[1]

describe('standalone comparison page', () => {
  it('renders canvas and Mermaid without network access', async () => {
    const fetch = vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('API unavailable'))
    let renderer!: ReactTestRenderer
    try {
      await act(async () => { renderer = create(<MemoryRouter initialEntries={['/compare' + hash]}><ComparisonScenePage /></MemoryRouter>) })
      // Native decompression finishes outside the initial React render.
      await vi.waitFor(() => expect(captured.embedded).toBeDefined())
      const embedded = captured.embedded as { scene: { views: object }; markdown: string }
      expect(Object.keys(embedded.scene.views)).toHaveLength(2)
      expect(embedded.markdown).toContain('```mermaid\nflowchart LR')
      expect(renderer.root.findAll((node) => node.type === 'div' && node.props.href?.startsWith('tld://compare#v1='))).toHaveLength(1)
      expect(fetch).not.toHaveBeenCalled()
    } finally { await act(async () => renderer?.unmount()); fetch.mockRestore() }
  })

  it('shows a readable error for invalid links', async () => {
    let renderer!: ReactTestRenderer
    await act(async () => { renderer = create(<MemoryRouter initialEntries={['/compare#v2=bad']}><ComparisonScenePage /></MemoryRouter>) })
    expect(renderer.root.findAll((node) => node.type === 'div' && node.props.role === 'alert')[0].children.join('')).toContain('Unsupported')
    await act(async () => renderer.unmount())
  })
})
