import React from 'react'
import { act, create } from 'react-test-renderer'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ImpactCommit, ImpactCommitDetails } from '../api/client'
import CommitHistoryPanel from './CommitGraph'

const apiMocks = vi.hoisted(() => ({
  listCommitGraph: vi.fn(),
  getCommitDetails: vi.fn(),
  getRangeStats: vi.fn(),
}))

vi.mock('../api/client', () => ({
  api: {
    impact: {
      listCommitGraph: apiMocks.listCommitGraph,
      getCommitDetails: apiMocks.getCommitDetails,
      getRangeStats: apiMocks.getRangeStats,
    },
  },
}))

vi.mock('../utils/toast', () => ({ toast: vi.fn() }))

vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react')
  const BoxLike = ({ children, ...props }: { children?: React.ReactNode }) =>
    ReactModule.createElement('div', props, children)
  const ButtonLike = ({ children, onClick, ...props }: { children?: React.ReactNode; onClick?: (event: unknown) => void }) =>
    ReactModule.createElement('button', { ...props, onClick }, children)
  const InputLike = ReactModule.forwardRef<HTMLInputElement, Record<string, unknown>>((props, ref) =>
    ReactModule.createElement('input', { ...props, ref }),
  )
  InputLike.displayName = 'InputLike'
  return {
    Badge: BoxLike,
    Box: BoxLike,
    Button: ButtonLike,
    Code: BoxLike,
    Collapse: BoxLike,
    Divider: BoxLike,
    Flex: BoxLike,
    HStack: BoxLike,
    IconButton: ButtonLike,
    Input: InputLike,
    InputGroup: BoxLike,
    InputLeftElement: BoxLike,
    InputRightElement: BoxLike,
    Progress: BoxLike,
    Select: BoxLike,
    Spinner: BoxLike,
    Tab: BoxLike,
    TabList: BoxLike,
    TabPanel: BoxLike,
    TabPanels: BoxLike,
    Tabs: BoxLike,
    Text: BoxLike,
    Tooltip: ({ children }: { children?: React.ReactNode }) => ReactModule.createElement(ReactModule.Fragment, {}, children),
    VStack: BoxLike,
  }
})

function graphCommit(sha: string, parents: string[] = [], subject = sha): ImpactCommit {
  return {
    sha,
    short_sha: sha.slice(0, 7),
    subject,
    author: 'Test',
    date: '2024-01-01',
    parents,
    refs: [],
    author_email: 'test@example.com',
    body: '',
  }
}

const HISTORY: ImpactCommit[] = [
  graphCommit('c3', ['c2'], 'third commit'),
  graphCommit('c2', ['c1'], 'second commit'),
  graphCommit('c1', [], 'first commit'),
]

const DETAILS: ImpactCommitDetails = {
  commit: HISTORY[1],
  files: [{ path: 'b.txt', change: 'added', added: 3, removed: 0 }],
  added: 3,
  removed: 0,
}

async function renderPanel(base = '', head = '') {
  const onSelectBase = vi.fn()
  const onSelectHead = vi.fn()
  const onRun = vi.fn()
  let root!: ReturnType<typeof create>
  await act(async () => {
    root = create(
      <CommitHistoryPanel
        path="/repo"
        base={base}
        head={head}
        onSelectBase={onSelectBase}
        onSelectHead={onSelectHead}
        onRun={onRun}
        running={false}
      />,
    )
  })
  await act(async () => {})
  return { root, onSelectBase, onSelectHead, onRun }
}

function textNodes(root: ReturnType<typeof create>, text: string) {
  return root.root.findAll(
    (node) => typeof node.type === 'string' && node.props?.children === text,
  )
}

describe('CommitHistoryPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    apiMocks.listCommitGraph.mockResolvedValue(HISTORY)
    apiMocks.getCommitDetails.mockResolvedValue(DETAILS)
    apiMocks.getRangeStats.mockResolvedValue({ commits: 1, files_changed: 1, added: 3, removed: 0 })
  })

  it('renders graph rows and wires endpoint selection', async () => {
    const { root, onSelectBase, onSelectHead } = await renderPanel()
    expect(apiMocks.listCommitGraph).toHaveBeenCalledWith('/repo', 100)
    expect(textNodes(root, 'second commit')).toHaveLength(1)

    const baseButtons = root.root.findAllByType('button').filter((node) => node.props.children === 'Base')
    expect(baseButtons.length).toBeGreaterThan(0)
    await act(async () => {
      baseButtons[0].props.onClick({ stopPropagation: vi.fn() })
    })
    expect(onSelectBase).toHaveBeenCalledWith('c3')

    const headButtons = root.root.findAllByType('button').filter((node) => node.props.children === 'Head')
    await act(async () => {
      headButtons[0].props.onClick({ stopPropagation: vi.fn() })
    })
    expect(onSelectHead).toHaveBeenCalledWith('c3')
  })

  it('marks selected endpoints and loads details on row click', async () => {
    const { root } = await renderPanel('c1', 'c3')
    expect(textNodes(root, 'BASE')).toHaveLength(1)
    expect(textNodes(root, 'HEAD')).toHaveLength(1)

    const rows = root.root.findAll(
      (node) => typeof node.type === 'string' && node.props?.h === '32px' && typeof node.props?.onClick === 'function',
    )
    expect(rows.length).toBe(3)
    await act(async () => {
      rows[1].props.onClick()
    })
    await act(async () => {})
    expect(apiMocks.getCommitDetails).toHaveBeenCalledWith('/repo', 'c2')
    expect(textNodes(root, 'b.txt').length).toBeGreaterThan(0)
  })
})
