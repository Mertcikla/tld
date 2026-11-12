import { useEffect, useMemo, useReducer, useState } from 'react'
import { HStack, IconButton, Tooltip } from '@chakra-ui/react'
import { ExternalLinkIcon } from '@chakra-ui/icons'
import type { IndexedRepository } from '../api/client'
import { listIndexedRepositories, resolveElementRepository } from '../utils/repositoryResolver'
import { sourceActionTarget, type SourceElement } from '../utils/sourceActionTarget'
import { peekSubSource, requestSubSource, subscribeSubSources, type SubSource } from '../utils/subElementSource'
import { parseSourceLink } from '../utils/sourceLinks'
import { openExternalUrl } from '../lib/desktop'
import { vscodeBridge } from '../lib/vscodeBridge'
import OpenInEditorButton from './OpenInEditorButton'

export default function SourceLinkActions({ element, source }: { element: SourceElement; source?: SubSource }) {
  const { repo, repository_id, file_path } = element
  const language = element.language, branch = element.branch
  const link = useMemo(() => ({ repo, repository_id, file_path, language, branch }), [repo, repository_id, file_path, language, branch])
  const resolveSymbol = !source && parseSourceLink(file_path).anchor.kind === 'symbol'
  const [, refresh] = useReducer(value => value + 1, 0)
  useEffect(() => resolveSymbol ? subscribeSubSources(refresh) : undefined, [resolveSymbol])
  useEffect(() => { if (resolveSymbol) requestSubSource(link) }, [resolveSymbol, link])
  const [indexed, setIndexed] = useState<IndexedRepository | null>(null)
  useEffect(() => {
    let cancelled = false
    setIndexed(null)
    if (repo || repository_id) void listIndexedRepositories().then(repos => {
      if (!cancelled) setIndexed(resolveElementRepository({ repo, repository_id, file_path }, repos))
    }).catch(() => { /* Remote navigation remains available without an index. */ })
    return () => { cancelled = true }
  }, [repo, repository_id, file_path])
  const target = sourceActionTarget(element, indexed, source ?? (resolveSymbol ? peekSubSource(link) : undefined))
  return <HStack className="nodrag nopan" spacing={1} onPointerDown={event => event.stopPropagation()} onClick={event => event.stopPropagation()}>
    {target.githubUrl && <Tooltip label="Open in GitHub"><IconButton aria-label="Open in GitHub" size="xs" variant="ghost" icon={<svg width="15" height="15" viewBox="0 0 24 24" fill="currentColor"><path d="M12 0a12 12 0 0 0-3.8 23.4c.6.1.8-.3.8-.6v-2.2c-3.3.7-4-1.4-4-1.4-.5-1.3-1.3-1.6-1.3-1.6-1.1-.8.1-.8.1-.8 1.2.1 1.8 1.3 1.8 1.3 1.1 1.9 2.9 1.3 3.5 1 .1-.8.4-1.3.8-1.6-2.7-.3-5.5-1.3-5.5-5.9 0-1.3.5-2.4 1.2-3.2-.1-.3-.5-1.5.1-3.2 0 0 1-.3 3.3 1.2a11.4 11.4 0 0 1 6 0C17.3 4.9 18.3 5.2 18.3 5.2c.6 1.7.2 2.9.1 3.2.8.8 1.2 1.9 1.2 3.2 0 4.6-2.8 5.6-5.5 5.9.4.4.8 1.1.8 2.2v3.1c0 .3.2.7.8.6A12 12 0 0 0 12 0Z" /></svg>} onClick={() => openExternalUrl(target.githubUrl!)} /></Tooltip>}
    {target.editorPath && (window.__TLD_VSCODE__ ? <Tooltip label="Open in editor"><IconButton aria-label="Open in editor" size="xs" variant="ghost" icon={<ExternalLinkIcon />} onClick={() => vscodeBridge.postMessage({ type: 'open-file', filePath: target.editorPath, startLine: target.start, symbolName: target.anchor.kind === 'symbol' ? target.anchor.symbolName : undefined, symbolKind: target.anchor.kind === 'symbol' ? target.anchor.nodeType : undefined })} /></Tooltip>
      : <OpenInEditorButton repo={target.repo} repositoryId={target.repositoryId} filePath={target.editorPath} line={target.start} branch={element.branch} borderless iconOnly fallbackToGithub={false} />)}
    {element.url && <Tooltip label="Open URL"><IconButton aria-label="Open URL" size="xs" variant="ghost" icon={<ExternalLinkIcon />} onClick={() => openExternalUrl(element.url!)} /></Tooltip>}
  </HStack>
}
