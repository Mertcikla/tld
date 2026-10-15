import React from 'react'
import { act, create } from 'react-test-renderer'
import { describe, expect, it, vi } from 'vitest'
import type { RepositoryImpactScene } from '../api/client'
import RepositoryChangeHistory from './RepositoryChangeHistory'

vi.mock('@chakra-ui/react', () => {
  const Box = React.forwardRef<HTMLDivElement, { children?: React.ReactNode }>(({ children, ...props }, ref) => React.createElement('div', { ...props, ref }, children))
  const Button = ({ children, ...props }: { children?: React.ReactNode }) => React.createElement('button', props, children)
  return { Box, Button, Flex: Box, HStack: Box, Text: Box, Tooltip: Box }
})
vi.mock('@chakra-ui/icons', () => ({ ChevronRightIcon: () => null }))

const path = 'src/very/deep/directory/a-file-with-a-long-name.ts'
function scene(count = 1, version = 'v1'): RepositoryImpactScene {
  return {
    repositoryId: 'repo', version, tree: [], navigations: [],
    views: { 1: { placements: Array.from({ length: count }, (_, id) => ({ element_id: id, name: `Element ${id}`, file_path: path, tags: [] })), connectors: [] } },
    overlays: Object.fromEntries(Array.from({ length: count }, (_, id) => [id, { change: 'modified', path, linesAdded: version === 'v1' ? 1 : 2, symbols: [] }])),
  } as unknown as RepositoryImpactScene
}
function rows(renderer: ReturnType<typeof create>) {
  return renderer.root.findAll((node) => node.type === 'div' && node.props['data-history-path'] !== undefined)
}

describe('compact materialization log', () => {
  it('shows clickable changes inline when they fit and restores counts and expansion on resize', async () => {
    let width = 1000
    let resize = () => {}
    const disconnect = vi.fn()
    vi.stubGlobal('ResizeObserver', class {
      constructor(callback: () => void) { resize = callback }
      observe() {}
      disconnect = disconnect
    })
    const onFocus = vi.fn()
    let renderer!: ReturnType<typeof create>
    try {
      await act(async () => {
        renderer = create(<RepositoryChangeHistory scene={scene()} onFocus={onFocus} />, { createNodeMock: (element) => {
          if (element.props['data-testid'] === 'history-change-row') return { get clientWidth() { return width } }
          if (element.props['data-testid'] === 'history-change-measure') return { scrollWidth: 210 }
          if (element.props['data-testid'] === 'history-change-metadata') return { offsetWidth: 80 }
          return null
        } })
      })
      const buttons = () => renderer.root.findAllByType('button')
      const resources = () => buttons().filter((node) => node.props['aria-label']?.startsWith('Focus element modified:'))
      expect(renderer.root.findAllByProps({ 'data-testid': 'history-inline-changes' }).length).toBeGreaterThan(0)
      expect(resources()).toHaveLength(1)
      expect(buttons().some((node) => node.props['aria-label'] === `Expand changes for ${path}`)).toBe(false)
      await act(async () => resources()[0].props.onClick())
      expect(onFocus).toHaveBeenCalledWith([expect.objectContaining({ kind: 'element' })])
      await act(async () => { width = 400; resize() })
      expect(resources()).toHaveLength(0)
      expect(buttons().some((node) => node.props['aria-label'] === `Focus element changes in ${path}`)).toBe(true)
      await act(async () => buttons().find((node) => node.props['aria-label'] === `Expand changes for ${path}`)!.props.onClick())
      expect(resources()).toHaveLength(1)
      await act(async () => { width = 1000; resize() })
      expect(renderer.root.findAllByProps({ 'data-testid': 'history-change-details' })).toHaveLength(0)
      expect(resources()).toHaveLength(1)
    } finally {
      if (renderer) await act(async () => renderer.unmount())
      vi.unstubAllGlobals()
    }
    expect(disconnect).toHaveBeenCalled()
  })

  it('colors individual changes by action in the expanded overflow', async () => {
    const current = scene(3)
    current.overlays[0].change = 'removed'
    current.overlays[1].change = 'added'
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<RepositoryChangeHistory scene={current} onFocus={vi.fn()} />) })
    await act(async () => renderer.root.findAllByType('button').find((node) => node.props['aria-label'] === `Expand changes for ${path}`)!.props.onClick())
    const buttons = renderer.root.findAllByType('button')
    expect(buttons.find((node) => node.props['aria-label'] === 'Focus element removed: Element 0')?.props.color).toBe('red.200')
    expect(buttons.find((node) => node.props['aria-label'] === 'Focus element added: Element 1')?.props.color).toBe('green.200')
    expect(buttons.find((node) => node.props['aria-label'] === 'Focus element modified: Element 2')?.props.color).toBe('orange.200')
    await act(async () => renderer.unmount())
  })

  it('restarts the flash only for changed rows without remounting their controls', async () => {
    // Even pushes within the same millisecond must restart an existing row's flash.
    const now = vi.spyOn(Date, 'now').mockReturnValue(1000)
    let renderer!: ReturnType<typeof create>
    try {
      const first = scene(2)
      first.views[1].placements[1].file_path = 'src/unchanged.ts'
      first.overlays[1].path = 'src/unchanged.ts'
      await act(async () => { renderer = create(<RepositoryChangeHistory scene={first} onFocus={vi.fn()} />) })
      const row = (file: string) => rows(renderer).find((node) => node.props['data-history-path'] === file)!
      const flash = (file: string) => row(file).findByProps({ 'data-testid': 'history-change-flash' })
      const changedRow = row(path)
      const control = changedRow.findAllByType('button')[0]
      const firstFlash = flash(path)
      const unchangedFlash = flash('src/unchanged.ts')
      const next = structuredClone(first)
      next.version = 'v2'
      next.overlays[0].linesAdded = 3
      await act(async () => renderer.update(<RepositoryChangeHistory scene={next} onFocus={vi.fn()} />))
      expect(row(path)).toBe(changedRow)
      expect(row(path).findAllByType('button')[0]).toBe(control)
      expect(flash(path)).not.toBe(firstFlash)
      expect(flash('src/unchanged.ts')).toBe(unchangedFlash)
      const updatedFlash = flash(path)
      await act(async () => renderer.update(<RepositoryChangeHistory scene={{ ...next, version: 'v3' }} onFocus={vi.fn()} />))
      expect(flash(path)).toBe(updatedFlash)
    } finally {
      if (renderer) await act(async () => renderer.unmount())
      now.mockRestore()
    }
  })

  it('collapses repeated edits into the same file row and keeps its focus action current', async () => {
    const onFocus = vi.fn()
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<RepositoryChangeHistory scene={scene()} onFocus={onFocus} />) })
    expect(rows(renderer)).toHaveLength(1)
    await act(async () => renderer.update(<RepositoryChangeHistory scene={scene(1, 'v2')} onFocus={onFocus} />))
    expect(rows(renderer)).toHaveLength(1)
    expect(rows(renderer)[0].findAll((node) => node.props.title === '2 updates to this file').length).toBeGreaterThan(0)
    await act(async () => renderer.root.findAllByType('button').find((node) => node.props['aria-label'] === `Focus element changes in ${path}`)!.props.onClick())
    expect(onFocus).toHaveBeenCalledWith([expect.objectContaining({ kind: 'element', elementIds: [{ viewId: 1, elementId: 0 }] })])
    await act(async () => renderer.unmount())
  })

  it('paginates large changes while keeping one file row and every resource reachable', async () => {
    const onFocus = vi.fn()
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<RepositoryChangeHistory scene={scene(1000)} onFocus={onFocus} />) })
    expect(rows(renderer)).toHaveLength(1)
    const buttons = () => renderer.root.findAllByType('button')
    const resourceButtons = () => buttons().filter((node) => node.props['aria-label']?.startsWith('Focus element modified:'))
    expect(resourceButtons()).toHaveLength(0)
    await act(async () => buttons().find((node) => node.props['aria-label'] === `Expand changes for ${path}`)!.props.onClick())
    expect(resourceButtons()).toHaveLength(12)
    const firstPage = resourceButtons().map((node) => node.props['aria-label'])
    await act(async () => buttons().find((node) => node.children.includes('Next'))!.props.onClick())
    expect(resourceButtons()).toHaveLength(12)
    expect(resourceButtons()[0].props['aria-label']).not.toBe(firstPage[0])
    await act(async () => resourceButtons()[0].props.onClick())
    expect(onFocus.mock.calls[0][0]).toHaveLength(1)
    await act(async () => renderer.unmount())
  })

  it('resets history when a different repository is displayed', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => { renderer = create(<RepositoryChangeHistory scene={scene()} />) })
    expect(rows(renderer)).toHaveLength(1)
    await act(async () => renderer.update(<RepositoryChangeHistory scene={{ ...scene(), repositoryId: 'other', overlays: {} }} />))
    expect(rows(renderer)).toHaveLength(0)
    await act(async () => renderer.unmount())
  })
})
