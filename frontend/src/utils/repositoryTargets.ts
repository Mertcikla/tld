import type {
  CodeSnapshot,
  RepositoryGitHistory,
  RepositoryMapOptions,
} from '../api/client'

export function snapshotForTarget(
  target: string,
  snapshots: CodeSnapshot[],
): CodeSnapshot | undefined {
  if (target.startsWith('snapshot:'))
    return snapshots.find((s) => s.id === target.slice(9))
  if (target.startsWith('commit:'))
    return [...snapshots]
      .reverse()
      .find(
        (s) => s.provenance === 'commit' && s.gitRevision === target.slice(7),
      )
  return undefined
}
export function defaultRepositoryTargets(
  snapshots: CodeSnapshot[],
  history: RepositoryGitHistory | null,
): [string, string] {
  if (snapshots.length >= 2)
    return [
      `snapshot:${snapshots[snapshots.length - 2].id}`,
      `snapshot:${snapshots[snapshots.length - 1].id}`,
    ]
  const head = history?.commits.find((c) => c.sha === history.headSha)
  return [
    head?.parents[0] ? `commit:${head.parents[0]}` : '',
    history?.headSha ? `commit:${history.headSha}` : '',
  ]
}
export function targetMapOptions(
  target: string,
  branch: string,
): RepositoryMapOptions {
  if (target.startsWith('snapshot:')) return { snapshotId: target.slice(9) }
  if (target.startsWith('commit:'))
    return { gitRevision: target.slice(7), gitBranch: branch }
  if (target === 'working_tree') return { workingTree: true }
  throw new Error('Select a snapshot, commit, or Working tree')
}
