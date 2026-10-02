import React from 'react'
import { act, create } from 'react-test-renderer'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import Repositories from './Repositories'

const { navigateMock } = vi.hoisted(() => ({ navigateMock: vi.fn() }))

vi.mock('react-router-dom', () => ({ useNavigate: () => navigateMock }))

vi.mock('../api/client', () => ({
  api: {
    repositories: {
      list: vi.fn(async () => [{
        id: 'repo-1',
        root: '/repo/demo',
        latestSnapshotId: 'snap-1',
        latestCreatedUnix: 100,
        gitRevision: 'abc',
        gitBranch: 'main',
        facts: 4,
        chunks: 4,
        edges: 0,
        sources: 4,
      }]),
      snapshots: vi.fn(async () => []),
      diff: vi.fn(async () => null),
      map: vi.fn(async (_repositoryId: string, handlers?: { onProgress?: (progress: { stage: string; current: number; total: number; detail: string }) => void }) => {
        handlers?.onProgress?.({ stage: 'clustering', current: 1, total: 2, detail: 'grow' })
        return { runId: 'run-1', viewId: 5, facts: 4, clusters: 1, bins: 1, unclustered: 0, weightedTightness: 1 }
      }),
    },
  },
}))

vi.mock('../utils/toast', () => ({ toast: vi.fn() }))

vi.mock('@chakra-ui/icons', () => ({
  CopyIcon: () => null,
  RepeatIcon: () => null,
}))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  type NodeProps = { children?: React.ReactNode; isOpen?: boolean }
  const BoxLike = ({ children, isOpen, ...props }: NodeProps) => (isOpen === false ? null : ReactModule.createElement('div', props, children))
  const ButtonLike = ({ children, onClick, isLoading, loadingText, isDisabled, ...props }: {
    children?: React.ReactNode
    onClick?: () => void
    isLoading?: boolean
    loadingText?: string
    isDisabled?: boolean
  }) => ReactModule.createElement('button', { ...props, disabled: isDisabled, onClick }, isLoading && loadingText ? loadingText : children)
  return {
    Alert: BoxLike,
    AlertIcon: BoxLike,
    Badge: BoxLike,
    Box: BoxLike,
    Button: ButtonLike,
    Center: BoxLike,
    Code: BoxLike,
    Divider: BoxLike,
    Flex: BoxLike,
    Grid: BoxLike,
    HStack: BoxLike,
    IconButton: ButtonLike,
    Progress: BoxLike,
    Select: BoxLike,
    Spinner: BoxLike,
    Text: BoxLike,
    Tooltip: BoxLike,
    VStack: BoxLike,
  }
})

describe('Repositories map action', () => {
  beforeEach(() => {
    navigateMock.mockClear()
  })

  it('runs the mapper and navigates to the materialized view', async () => {
    let renderer!: ReturnType<typeof create>
    await act(async () => {
      renderer = create(<Repositories />)
    })

    const mapButton = renderer.root.findByProps({ 'data-testid': 'repositories-map' })
    await act(async () => {
      await mapButton.props.onClick()
    })

    const { api } = await import('../api/client')
    expect(api.repositories.map).toHaveBeenCalledWith('repo-1', expect.objectContaining({ onProgress: expect.any(Function) }))
    expect(navigateMock).toHaveBeenCalledWith('/views/5')
  })
})
