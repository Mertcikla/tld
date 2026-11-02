import { beforeEach, describe, expect, it, vi } from 'vitest'
import { peekSubSource, requestSubSource, subscribeSubSources } from './subElementSource'

const mocks = vi.hoisted(() => ({ source: vi.fn(), symbols: vi.fn(), find: vi.fn() }))
vi.mock('../api/client', () => ({ api: { editor: { source: mocks.source } } }))
vi.mock('./repositoryResolver', () => ({ listIndexedRepositories: async () => [], resolveElementRepository: () => null }))
vi.mock('./treesitter', () => ({
  detectLanguage: () => 'typescript',
  getParser: async () => ({ parse: () => ({ delete: vi.fn() }) }),
  extractSymbols: mocks.symbols,
  findSymbolByName: mocks.find,
}))
vi.mock('./subElementSyntax', () => ({ highlightSource: async () => [] }))

beforeEach(() => {
  mocks.source.mockReset().mockResolvedValue({ content: '// context\nfunction find() {\n  return 1\n}\n', path: 'users.ts' })
  mocks.symbols.mockReturnValue([{ name: 'find', startLine: 2, endLine: 4 }])
  mocks.find.mockReturnValue({ name: 'find', startLine: 2, endLine: 4 })
})

describe('cached source documents', () => {
  it('deduplicates loads and preserves file line numbers for range handles', async () => {
    const link = { repo: '/range-repo', file_path: 'users.ts#L2-L4' }
    const stop = subscribeSubSources(vi.fn())
    const state = requestSubSource(link)
    expect(requestSubSource(link)).toBe(state)
    await vi.waitFor(() => expect(state.status).toBe('ready'))
    expect(state.lines).toEqual(['function find() {', '  return 1', '}'])
    expect([state.startLine, state.endLine]).toEqual([2, 4])
    await vi.waitFor(() => expect(state.symbols).toHaveLength(1))
    expect(mocks.source).toHaveBeenCalledTimes(1)
    expect(peekSubSource(link)).toBe(state)
    stop()
  })
  it('resolves named symbols and reports missing symbols', async () => {
    const state = requestSubSource({ repo: '/symbol-repo', file_path: 'users.ts#function_declaration:find' })
    await vi.waitFor(() => expect(state.status).toBe('ready'))
    expect([state.startLine, state.endLine]).toEqual([2, 4])
    mocks.find.mockReturnValue(null)
    const missing = requestSubSource({ repo: '/missing-repo', file_path: 'users.ts#function_declaration:gone' })
    await vi.waitFor(() => expect(missing.status).toBe('error'))
    expect(missing.error).toContain('was not found')
  })
  it('retries failures without retaining stale source content', async () => {
    const link = { repo: '/retry-repo', file_path: 'users.ts' }
    mocks.source.mockRejectedValueOnce(new Error('Unavailable'))
    const failed = requestSubSource(link)
    await vi.waitFor(() => expect(failed.status).toBe('error'))
    const retry = requestSubSource(link, true)
    expect(retry).not.toBe(failed)
    await vi.waitFor(() => expect(retry.status).toBe('ready'))
    expect(peekSubSource(link)).toBe(retry)
  })
})
