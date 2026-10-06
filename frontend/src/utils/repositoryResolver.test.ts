import { describe, expect, it } from 'vitest'
import type { IndexedRepository } from '../api/client'
import {
  normalizeRemoteKey,
  parseRepositoryAddInput,
  resolveElementRepository,
} from './repositoryResolver'

function repo(overrides: Partial<IndexedRepository> & { id: string }): IndexedRepository {
  return {
    root: '/work/repo',
    latestSnapshotId: 'snap',
    latestCreatedUnix: 0,
    gitRevision: '',
    gitBranch: '',
    facts: 0,
    edges: 0,
    sources: 0,
    remoteUrl: '',
    name: 'repo',
    managed: false,
    ...overrides,
  }
}

describe('normalizeRemoteKey', () => {
  it('normalizes https, ssh, scp, and shorthand forms', () => {
    expect(normalizeRemoteKey('https://github.com/Owner/Repo.git')).toBe('github.com/owner/repo')
    expect(normalizeRemoteKey('git@github.com:Owner/Repo.git')).toBe('github.com/owner/repo')
    expect(normalizeRemoteKey('Owner/Repo')).toBe('github.com/owner/repo')
    expect(normalizeRemoteKey('https://gitlab.com/group/sub/repo')).toBe('gitlab.com/group/sub/repo')
  })

  it('rejects local paths and empty values', () => {
    expect(normalizeRemoteKey('/work/repo')).toBe('')
    expect(normalizeRemoteKey('~/repo')).toBe('')
    expect(normalizeRemoteKey('')).toBe('')
  })
})

describe('resolveElementRepository', () => {
  const repos = [
    repo({ id: 'alpha', root: '/work/alpha', remoteUrl: 'https://github.com/owner/alpha' }),
    repo({ id: 'beta', root: '/work/beta', remoteUrl: '' }),
  ]

  it('prefers the explicit repository id', () => {
    expect(resolveElementRepository({ repository_id: 'beta', repo: 'owner/alpha' }, repos)?.id).toBe('beta')
  })

  it('matches the legacy repo remote value', () => {
    expect(resolveElementRepository({ repo: 'owner/alpha' }, repos)?.id).toBe('alpha')
    expect(resolveElementRepository({ repo: 'git@github.com:Owner/Alpha.git' }, repos)?.id).toBe('alpha')
  })

  it('matches a local root or absolute file path', () => {
    expect(resolveElementRepository({ repo: '/work/beta' }, repos)?.id).toBe('beta')
    expect(resolveElementRepository({ file_path: '/work/alpha/src/main.go#L4' }, repos)?.id).toBe('alpha')
  })

  it('returns null when nothing matches', () => {
    expect(resolveElementRepository({ repo: 'owner/missing' }, repos)).toBeNull()
    expect(resolveElementRepository({ repository_id: 'missing' }, repos)).toBeNull()
  })
})

describe('parseRepositoryAddInput', () => {
  it('detects remote references', () => {
    expect(parseRepositoryAddInput('facebook/react')).toEqual({ remoteUrl: 'facebook/react' })
    expect(parseRepositoryAddInput('https://github.com/facebook/react')).toEqual({ remoteUrl: 'https://github.com/facebook/react' })
    expect(parseRepositoryAddInput('git@github.com:facebook/react.git')).toEqual({ remoteUrl: 'git@github.com:facebook/react.git' })
  })

  it('keeps local paths', () => {
    expect(parseRepositoryAddInput('/work/repo')).toEqual({ path: '/work/repo' })
    expect(parseRepositoryAddInput('./some/repo')).toEqual({ path: './some/repo' })
  })
})
