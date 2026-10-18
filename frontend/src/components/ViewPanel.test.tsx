import React from 'react'
import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api } from '../api/client'
import type { ViewTreeNode } from '../types'
import ViewPanel from './ViewPanel'

vi.mock('../api/client', () => ({ api: { workspace: { views: { update: vi.fn() } } } }))
vi.mock('./SlidingPanel', () => ({ default: ({ children }: { children?: React.ReactNode }) => <>{children}</> }))
vi.mock('./PanelHeader', () => ({ default: () => null }))
vi.mock('./LayoutSection', () => ({ default: () => null }))
vi.mock('./TagUpsert', () => ({ default: () => null }))
vi.mock('./ScrollIndicatorWrapper', () => ({ default: ({ children }: { children?: React.ReactNode }) => <>{children}</> }))
vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const passthrough = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children)
  return {
    ...Object.fromEntries(['Box', 'Button', 'Checkbox', 'Collapse', 'Divider', 'FormControl', 'FormLabel', 'HStack', 'Icon', 'Input', 'Tag', 'TagCloseButton', 'TagLabel', 'Text', 'Textarea', 'VStack', 'Wrap', 'WrapItem'].map((name) => [name, passthrough])),
    useBreakpointValue: () => false,
    useToast: () => vi.fn(),
  }
})

const view: ViewTreeNode = {
  id: 69, name: 'Workspace', description: null, level_label: null, tags: [], level: 0, depth: 0,
  created_at: '2026-10-18', updated_at: '2026-10-18', parent_view_id: null, children: [],
}
let renderer: ReactTestRenderer

beforeEach(() => {
  vi.useFakeTimers()
  vi.mocked(api.workspace.views.update).mockReset()
  vi.stubGlobal('window', { addEventListener: vi.fn(), removeEventListener: vi.fn() })
})
afterEach(() => {
  act(() => renderer?.unmount())
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

function renderPanel(canEdit = true) {
  const props = { isOpen: true, onClose: vi.fn(), onSave: vi.fn(), view, canEdit }
  act(() => { renderer = create(<ViewPanel {...props} />) })
  return props
}
const field = () => renderer.root.findByProps({ 'data-testid': 'view-panel-name-input' })
const saveButton = () => renderer.root.findByProps({ 'data-testid': 'view-panel-save' })

describe('ViewPanel saving', () => {
  it('keeps a failed draft open, shows an error, and retries the same values', async () => {
    const props = renderPanel()
    expect(renderer.root.findAllByProps({ 'data-testid': 'panel-save-status' })).toHaveLength(0)
    act(() => { field().props.onChange({ target: { value: 'Updated workspace' } }) })
    vi.mocked(api.workspace.views.update).mockRejectedValueOnce(new Error('offline'))
    await act(async () => { await saveButton().props.onClick() })
    expect(props.onSave).not.toHaveBeenCalled()
    expect(props.onClose).not.toHaveBeenCalled()
    expect(field().props.value).toBe('Updated workspace')
    expect(renderer.root.findByProps({ 'data-testid': 'panel-save-status' }).props.role).toBe('alert')

    vi.mocked(api.workspace.views.update).mockResolvedValueOnce({ id: 69, name: 'Updated workspace', label: null, tags: [], is_root: true, owner_element_id: null, created_at: '', updated_at: '' })
    await act(async () => { await saveButton().props.onClick() })
    expect(api.workspace.views.update).toHaveBeenLastCalledWith(69, { name: 'Updated workspace', description: '', label: '', tags: [] })
    expect(props.onSave).toHaveBeenCalledWith(expect.objectContaining({ name: 'Updated workspace' }))
    expect(props.onClose).toHaveBeenCalledOnce()
  })

  it('shows pending feedback and rejects another save while the request is in progress', async () => {
    renderPanel()
    let resolve!: (value: Awaited<ReturnType<typeof api.workspace.views.update>>) => void
    vi.mocked(api.workspace.views.update).mockImplementationOnce(() => new Promise((done) => { resolve = done }))
    act(() => { void saveButton().props.onClick() })
    act(() => { vi.advanceTimersByTime(300) })
    expect(saveButton().props.isLoading).toBe(true)
    expect(renderer.root.findByProps({ 'data-testid': 'panel-save-status' }).props.children).toContain('Saving changes…')
    await act(async () => { await saveButton().props.onClick() })
    expect(api.workspace.views.update).toHaveBeenCalledOnce()
    await act(async () => { resolve({ id: 69, name: 'Workspace', label: null, tags: [], is_root: true, owner_element_id: null, created_at: '', updated_at: '' }) })
  })

  it('does not display editing controls in a read-only panel', () => {
    renderPanel(false)
    expect(renderer.root.findAllByProps({ 'data-testid': 'panel-save-status' })).toHaveLength(0)
    expect(renderer.root.findAllByProps({ 'data-testid': 'view-panel-save' })).toHaveLength(0)
  })
})
