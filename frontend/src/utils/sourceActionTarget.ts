import type { IndexedRepository } from '../api/client'
import type { PlacedElement } from '../types'
import { normalizeRemoteKey } from './repositoryResolver'
import { githubBranchName, parseSourceLink, sourceLinkKind } from './sourceLinks'
import type { SubSource } from './subElementSource'

export type SourceElement = Pick<PlacedElement, 'repo' | 'repository_id' | 'file_path' | 'branch' | 'url' | 'language'>
export function sourceActionTarget(element: SourceElement, indexedRepo?: IndexedRepository | null, source?: SubSource) {
  const { basePath, anchor } = parseSourceLink(element.file_path)
  const kind = sourceLinkKind(element.file_path)
  const remote = normalizeRemoteKey(indexedRepo?.remoteUrl || element.repo)
  const root = remote.startsWith('github.com/') ? `https://github.com/${remote.slice('github.com/'.length)}` : null
  const branch = githubBranchName(element.branch || indexedRepo?.gitBranch) || 'main'
  const start = anchor.kind === 'line' ? anchor.startLine : anchor.kind === 'symbol' && source?.status === 'ready' ? source.startLine : undefined
  const end = anchor.kind === 'line' ? anchor.endLine : start ? source?.endLine ?? start : undefined
  const path = basePath.replace(/[\\/]$/, '').split('/').map(encodeURIComponent).join('/')
  const githubUrl = root ? kind === 'repository' ? element.branch ? `${root}/tree/${encodeURIComponent(branch)}` : root
    : `${root}/${kind === 'file' ? 'blob' : 'tree'}/${encodeURIComponent(branch)}/${path}${kind === 'file' && start ? `#L${start}-L${end}` : ''}` : null
  const localRepo = /^(\/|\\\\|[A-Za-z]:[\\/])/.test(element.repo ?? '')
  return { githubUrl, basePath, anchor, start, editorPath: basePath || indexedRepo?.root || (localRepo ? element.repo ?? '' : ''), repositoryId: indexedRepo?.id ?? element.repository_id, repo: indexedRepo?.root ?? element.repo ?? '' }
}

