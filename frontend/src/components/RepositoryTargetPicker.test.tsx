import React from 'react'
import { act, create } from 'react-test-renderer'
import { afterEach, describe, expect, it, vi } from 'vitest'
import RepositoryTargetPicker from './RepositoryTargetPicker'

vi.mock('../api/client', () => ({ api: { repositories: { commitDetails: vi.fn() } } }))
vi.mock('@chakra-ui/icons', () => ({ ChevronDownIcon: () => null }))
vi.mock('@chakra-ui/react', () => {
  const Box = ({ children, as = 'div', ...props }: { children?: React.ReactNode; as?: string }) => React.createElement(as, props, children)
  return {
    Box, HStack: Box, Text: Box, MenuButton: Box, MenuItemOption: Box, MenuList: Box, MenuOptionGroup: Box, Portal: Box,
    Menu: (props: Record<string, unknown>) => React.createElement(Box, { ...props, 'data-testid': 'menu' }),
    Input: (props: Record<string, unknown>) => React.createElement('input', props),
    Button: ({ children, ...props }: { children?: React.ReactNode }) => React.createElement('button', props, children),
  }
})

const sha = 'abcdef1234567890abcdef1234567890abcdef1234'
const details = { commit: { sha, subject: 'A commit', body: '', author: '', authorEmail: '', createdUnix: 1, parents: [], refs: [] }, files: [] }
const props = {
  repositoryId: 'repo', value: 'working_tree', card: true, 'aria-label': 'Base target',
  groups: [{ options: [{ value: 'working_tree', label: 'Working tree' }] }, { label: 'Saved snapshots', options: [{ value: 'snapshot:1', label: 'Snapshot one' }] }],
}
function renderPicker(onChange = vi.fn()) {
  let renderer!: ReturnType<typeof create>
  act(() => { renderer = create(<RepositoryTargetPicker {...props} onChange={onChange} />) })
  const menu = () => renderer.root.findByProps({ 'data-testid': 'menu' })
  act(() => { menu().props.onOpen() })
  const input = () => renderer.root.findByType('input')
  const enter = (value: string) => act(() => { input().props.onChange({ target: { value } }) })
  const submit = () => act(async () => { renderer.root.findByType('form').props.onSubmit({ preventDefault: vi.fn(), stopPropagation: vi.fn() }) })
  return { renderer, menu, input, enter, submit, onChange }
}

describe('RepositoryTargetPicker manual commits', () => {
  afterEach(() => { vi.resetAllMocks() })

  it.each([sha, '  ABCDEF1  '])('resolves a full or abbreviated hash and applies the canonical commit: %s', async (hash) => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.commitDetails).mockResolvedValueOnce(details)
    const picker = renderPicker()
    const entries = picker.renderer.root.findAll((node) => typeof node.type === 'string' && (node.type === 'form' || node.props.children === 'Saved snapshots'))
    expect(entries.map((node) => node.type)).toEqual(['form', 'div'])
    picker.enter(hash)
    await picker.submit()
    expect(api.repositories.commitDetails).toHaveBeenCalledWith('repo', hash.trim().toLowerCase())
    expect(picker.onChange).toHaveBeenCalledWith(`commit:${sha}`)
    expect(picker.menu().props.isOpen).toBe(false)
    act(() => { picker.renderer.unmount() })
  })

  it('keeps editing keys inside the field while allowing Escape and Tab to reach the menu', () => {
    const picker = renderPicker()
    for (const key of ['a', 'ArrowLeft', 'ArrowRight', 'Home', 'End', 'Enter', 'Escape', 'Tab']) {
      const stopPropagation = vi.fn()
      picker.input().props.onKeyDown({ key, stopPropagation })
      expect(stopPropagation).toHaveBeenCalledTimes(key === 'Escape' || key === 'Tab' ? 0 : 1)
    }
    act(() => { picker.renderer.unmount() })
  })

  it('reports invalid and unknown hashes without changing the selection or closing the menu', async () => {
    const { api } = await import('../api/client')
    const picker = renderPicker()
    picker.enter('not-a-hash')
    await picker.submit()
    expect(api.repositories.commitDetails).not.toHaveBeenCalled()
    expect(picker.renderer.root.findAllByProps({ role: 'alert' }).length).toBeGreaterThan(0)
    vi.mocked(api.repositories.commitDetails).mockRejectedValueOnce(new Error('Unknown commit'))
    picker.enter('badc0de')
    await picker.submit()
    expect(picker.renderer.root.findAllByProps({ role: 'alert' }).some((node) => node.props.children === 'Unknown commit')).toBe(true)
    expect(picker.onChange).not.toHaveBeenCalled()
    expect(picker.menu().props.isOpen).toBe(true)
    expect(picker.input().props.value).toBe('badc0de')
    act(() => { picker.renderer.unmount() })
  })

  it('keeps the menu open when the chosen commit would reverse the range', async () => {
    const { api } = await import('../api/client')
    vi.mocked(api.repositories.commitDetails).mockResolvedValueOnce(details)
    const picker = renderPicker(vi.fn(() => false))
    picker.enter('abcdef1')
    await picker.submit()
    expect(picker.menu().props.isOpen).toBe(true)
    expect(picker.renderer.root.findAllByProps({ role: 'alert' }).some((node) => node.props.children.includes('Base must precede Head'))).toBe(true)
    act(() => { picker.renderer.unmount() })
  })

  it('ignores a lookup that finishes after closing the menu', async () => {
    const { api } = await import('../api/client')
    let finish!: (value: typeof details) => void
    vi.mocked(api.repositories.commitDetails).mockImplementationOnce(() => new Promise((resolve) => { finish = resolve }))
    const picker = renderPicker()
    picker.enter('abcdef1')
    await picker.submit()
    act(() => { picker.menu().props.onClose(); picker.menu().props.onOpen() })
    await act(async () => { finish(details) })
    expect(picker.onChange).not.toHaveBeenCalled()
    expect(picker.input().props.value).toBe('')
    act(() => { picker.renderer.unmount() })
  })
})
