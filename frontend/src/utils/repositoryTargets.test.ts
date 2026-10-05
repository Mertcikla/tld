import { describe, expect, it } from 'vitest'
import type { CodeSnapshot, RepositoryGitHistory } from '../api/client'
import {
  defaultRepositoryTargets,
  snapshotForTarget,
  targetMapOptions,
} from './repositoryTargets'
const snapshot = (id: string, provenance: string): CodeSnapshot => ({
  id,
  provenance,
  repositoryId: 'repo',
  createdUnix: 1,
  gitRevision: 'sha',
  gitBranch: 'main',
  ingestionStatus: 'complete',
  projects: [],
  warnings: [],
  contentFingerprint: 'fp',
})
describe('repository targets', () => {
  it('uses two latest saved snapshots rather than forcing head to the repository latest pointer', () => {
    expect(
      defaultRepositoryTargets(
        [snapshot('one', ''), snapshot('two', ''), snapshot('three', '')],
        null,
      ),
    ).toEqual(['snapshot:two', 'snapshot:three'])
  })
  it('uses HEAD and its first parent when saved history is insufficient', () => {
    const history = {
      headSha: 'head',
      commits: [{ sha: 'head', parents: ['parent'] }],
    } as RepositoryGitHistory
    expect(defaultRepositoryTargets([], history)).toEqual([
      'commit:parent',
      'commit:head',
    ])
    expect(defaultRepositoryTargets([], null)).toEqual(['', ''])
  })
  it('does not match local edits or legacy captures to a clean commit', () => {
    expect(
      snapshotForTarget('commit:sha', [
        snapshot('legacy', ''),
        snapshot('local', 'working_tree'),
      ]),
    ).toBeUndefined()
    expect(
      snapshotForTarget('snapshot:local', [snapshot('local', 'working_tree')])
        ?.id,
    ).toBe('local')
    expect(
      snapshotForTarget('commit:sha', [snapshot('clean', 'commit')])?.id,
    ).toBe('clean')
  })
  it('sends mutually exclusive snapshot, commit, or working-tree inputs', () => {
    expect(targetMapOptions('snapshot:saved', 'main')).toEqual({
      snapshotId: 'saved',
    })
    expect(targetMapOptions('commit:sha', 'feature')).toEqual({
      gitRevision: 'sha',
      gitBranch: 'feature',
    })
    expect(targetMapOptions('working_tree', 'main')).toEqual({
      workingTree: true,
    })
    expect(() => targetMapOptions('', '')).toThrow()
  })
})
