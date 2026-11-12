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

  it.each([
    [' https://github.com/Owner/Repo.git/// ', 'github.com/owner/repo'],
    ['git@github.com:/Owner/Repo.git///', 'github.com/owner/repo'],
    ['Owner/Repo///', 'github.com/owner/repo'],
    ['https://github.com///Owner/Repo///?ref=main', 'github.com/owner/repo'],
    ['https://github.com////?ref=main', ''],
  ])('normalizes slash boundaries in %s', (input, expected) => {
    expect(normalizeRemoteKey(input)).toBe(expected)
  })

  it('handles long slash sequences with and without a trailing suffix', () => {
    const slashes = '/'.repeat(100_000)
    const path = `owner${slashes}repo`
    expect(normalizeRemoteKey(`github.com/${path}.git`)).toBe(`github.com/${path}`)
    expect(normalizeRemoteKey(`github.com/owner/repo.git${slashes}`)).toBe('github.com/owner/repo')
    expect(normalizeRemoteKey(`https://github.com/${path}?ref=main`)).toBe(`github.com/${path}`)
    expect(normalizeRemoteKey(`https://github.com/owner/repo${slashes}?ref=main`)).toBe('github.com/owner/repo')
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

  it('matches roots with trailing slashes while preserving path boundaries', () => {
    const slashes = '/'.repeat(100_000)
    const repositories = [repo({ id: 'alpha', root: `/work/alpha${slashes}` })]
    expect(resolveElementRepository({ file_path: '/work/alpha/src/main.go' }, repositories)?.id).toBe('alpha')
    expect(resolveElementRepository({ file_path: '/work/alphabet/src/main.go' }, repositories)).toBeNull()
    expect(resolveElementRepository({ file_path: '/work/file' }, [repo({ id: 'empty', root: slashes })])).toBeNull()

    const root = `/work${slashes}alpha`
    expect(resolveElementRepository({ file_path: `${root}/main.go` }, [repo({ id: 'nested', root })])?.id).toBe('nested')
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
