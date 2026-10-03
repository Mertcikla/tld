import { describe, expect, it } from 'vitest'
import type { RepositoryWatchStatus } from '../api/client'
import { watcherActivity } from './repositoryWatcher'

const status = (values: Partial<RepositoryWatchStatus>) => ({ running: true, state: 'idle', stage: '', debounceMs: 500, ...values }) as RepositoryWatchStatus

describe('watcher activity', () => {
  it('shows idle, debounce and actual indexing stages distinctly', () => {
    expect(watcherActivity(status({})).label).toBe('Idle · watching for changes')
    expect(watcherActivity(status({ state: 'debouncing' })).label).toBe('Debouncing changes')
    expect(watcherActivity(status({ state: 'scanning', stage: 'tree-sitter' })).label).toBe('Parsing sources')
    expect(watcherActivity(status({ state: 'scanning', stage: 'scip' })).label).toBe('Indexing symbols')
  })
  it('distinguishes work from waiting on the indexer or embedding service', () => {
    expect(watcherActivity(status({ stage: 'waiting-indexer' })).label).toBe('Waiting for indexer')
    expect(watcherActivity(status({ stage: 'embedding' })).label).toBe('Embedding symbols')
    expect(watcherActivity(status({ stage: 'waiting-embedding' })).label).toBe('Waiting for embedding service')
    expect(watcherActivity(status({ stage: 'live-map' })).label).toBe('Updating live map')
  })
  it('prioritizes lifecycle state over stale pipeline stages', () => {
    expect(watcherActivity(status({ running: false, stage: 'embedding' })).label).toBe('Stopped')
    expect(watcherActivity(status({ stopRequested: true, stage: 'embedding' })).label).toBe('Stopping…')
    expect(watcherActivity(status({ state: 'error', stage: 'discover' })).label).toBe('Retrying after an error')
    expect(watcherActivity(null).label).toBe('Checking watcher…')
  })
})
