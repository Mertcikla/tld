import { api, type RepositorySourceDiffLine } from '../api/client'
import type { LibraryElement, PlacedElement } from '../types'
import { githubBranchName, parseSourceLink } from './sourceLinks'
import { parseRepoSlug } from './url'
import { listIndexedRepositories, resolveElementRepository } from './repositoryResolver'
import type { SubSymbol } from './subElements'
import { vscodeBridge } from '../lib/vscodeBridge'
import { protobufSymbols } from './protobufSymbols'

export interface SourceToken { text: string; color: string }
export type SourceSymbol = SubSymbol
export interface SubSource {
  diff?: { lines: RepositorySourceDiffLine[]; side: 'base' | 'head'; baseTokens: SourceToken[][]; headTokens: SourceToken[][] }
  status: 'loading' | 'ready' | 'error'
  error?: string
  lines: string[]
  tokens: SourceToken[][]
  symbols: SourceSymbol[]
  startLine: number
  endLine: number
}
export type SubSourceLink = Pick<LibraryElement | PlacedElement, 'repo' | 'repository_id' | 'branch' | 'file_path' | 'language'> & { comparisonKey?: string; comparisonSide?: 'base' | 'head'; comparisonDiff?: boolean }
type Link = SubSourceLink
const cache = new Map<string, SubSource>()
const listeners = new Set<() => void>()
export const subscribeSubSources = (fn: () => void) => { listeners.add(fn); return () => { listeners.delete(fn) } }
export const subSourceKey = (link: Link) => JSON.stringify([link.repository_id, link.repo, link.branch, link.file_path, link.language, link.comparisonKey, link.comparisonSide, link.comparisonDiff])
export function peekSubSource(link: Link) { return cache.get(subSourceKey(link)) }
const notify = () => listeners.forEach(fn => fn())

async function loadText(link: Link, path: string): Promise<string> {
  if (link.comparisonKey) {
    if (!link.repository_id) throw new Error('A repository is required for captured source')
    return (await api.repositories.impactSource(link.repository_id, link.comparisonKey, path, link.comparisonSide ?? 'head')).content
  }
  if (typeof window !== 'undefined' && window.__TLD_VSCODE_API__) {
    const chunk = (offset: number): Promise<{ content: string; start: number }> => new Promise((resolve, reject) => {
      const requestId = `sub-source-${Math.random().toString(36).slice(2)}`
      const timer = setTimeout(() => { unsub(); reject(new Error('Source request timed out')) }, 15000)
      const unsub = vscodeBridge.onMessage(msg => {
        if (msg.type !== 'file-content' || msg.requestId !== requestId) return
        clearTimeout(timer); unsub()
        resolve({ content: msg.content, start: msg.startLineOffset })
      })
      // Existing extension hosts expose 10 lines of context on either side.
      vscodeBridge.postMessage({ type: 'request-file-content', requestId, filePath: path, startLine: offset + 10 })
    })
    const lines: string[] = []
    while (lines.length < 100000) {
      const next = await chunk(lines.length)
      const part = next.content.split('\n')
      if (next.start !== lines.length) throw new Error('Source changed while reading; reload the card')
      if (next.content === '' && lines.length > 0) return lines.join('\n')
      lines.push(...part)
      if (part.length < 21) return lines.join('\n')
    }
    throw new Error('Source exceeds the 100,000 line preview limit')
  }
  const repos = await listIndexedRepositories().catch(() => [])
  const indexed = resolveElementRepository(link, repos)
  if (indexed || link.repository_id || /^(\/|[A-Za-z]:[\\/])/.test(link.repo ?? '')) {
    return (await api.editor.source({ repository_id: indexed?.id ?? link.repository_id, repo: indexed?.root ?? link.repo, file_path: path })).content
  }
  const slug = parseRepoSlug(link.repo ?? '')
  if (!slug) throw new Error('Link a source file to display code')
  const branch = githubBranchName(link.branch) || 'main'
  const url = `https://raw.githubusercontent.com/${slug}/refs/heads/${branch}/${path.split('/').map(encodeURIComponent).join('/')}`
  const response = await fetch(url)
  if (!response.ok) throw new Error(`Source unavailable (${response.status})`)
  return response.text()
}

export function requestSubSource(link: Link, retry = false): SubSource {
  const key = subSourceKey(link)
  const existing = cache.get(key)
  if (existing && !retry) return existing
  const result: SubSource = { status: 'loading', lines: [], tokens: [], symbols: [], startLine: 1, endLine: 1 }
  cache.set(key, result)
  // Bound retained documents; no source text is written to the element or database.
  if (cache.size > 512) cache.delete(cache.keys().next().value!)
  void (async () => {
    try {
      const { basePath, anchor } = parseSourceLink(link.file_path)
      if (!basePath || /[\\/]$/.test(basePath)) throw new Error('Link a source file to display code')
      const capturedDiff = link.comparisonDiff && link.comparisonKey && link.repository_id
        ? await api.repositories.impactSource(link.repository_id, link.comparisonKey, basePath, link.comparisonSide ?? 'head', true) : null
      const text = capturedDiff?.content ?? await loadText(link, basePath)
      if (capturedDiff) result.diff = { lines: capturedDiff.diffLines, side: link.comparisonSide ?? 'head', baseTokens: [], headTokens: [] }
      const lines = text.split('\n')
      let start = anchor.kind === 'line' ? anchor.startLine : 1
      let end = anchor.kind === 'line' ? anchor.endLine : lines.length
      const setRange = () => {
        start = Math.max(1, Math.min(lines.length, start)); end = Math.max(start, Math.min(lines.length, end))
        result.lines = lines.slice(start - 1, end)
        result.startLine = start; result.endLine = end; result.status = 'ready'
        notify()
      }
      const highlight = async () => {
        try {
          const { highlightSource } = await import('./subElementSyntax')
          result.tokens = await highlightSource(text, basePath, start, end)
          if (capturedDiff && result.diff) {
            const [baseTokens, headTokens] = await Promise.all([
              highlightSource(capturedDiff.baseContent, basePath, 1, capturedDiff.baseContent.split('\n').length),
              highlightSource(capturedDiff.headContent, basePath, 1, capturedDiff.headContent.split('\n').length),
            ])
            result.diff = { ...result.diff, baseTokens, headTokens }
          }
          notify()
        } catch { /* Plain text remains usable if an optional grammar cannot load. */ }
      }
      if (anchor.kind !== 'symbol') { setRange(); void highlight() }
      const { detectLanguage, getParser, extractSymbols, findSymbolByName } = await import('./treesitter')
      const language = detectLanguage(basePath)
      let symbols: SourceSymbol[] = []
      if (/\.proto$/i.test(basePath)) {
        symbols = protobufSymbols(text)
        if (anchor.kind === 'symbol') {
          const found = symbols.find(symbol => (symbol.name === anchor.symbolName || symbol.qualifiedName === anchor.symbolName) && (!anchor.nodeType || symbol.type === anchor.nodeType))
          if (!found) throw new Error(`Symbol “${anchor.symbolName}” was not found`)
          start = found.startLine; end = found.endLine
        }
      } else if (language) {
        try {
          const parser = await getParser(language)
          const tree = parser.parse(text)
          try {
            symbols = extractSymbols(tree, language)
            if (anchor.kind === 'symbol') {
              const found = findSymbolByName(tree, language, anchor.symbolName, anchor.nodeType)
              if (found) { start = found.startLine; end = found.endLine }
              else throw new Error(`Symbol “${anchor.symbolName}” was not found`)
            }
          } finally { tree.delete() }
        } catch (error) {
          if (anchor.kind === 'symbol') throw error
        }
      } else if (anchor.kind === 'symbol') throw new Error('Symbol lookup is unavailable for this language; use a line range')
      if (anchor.kind === 'symbol') { setRange(); void highlight() }
      result.symbols = symbols.filter(s => s.startLine >= start && s.endLine <= end)
      notify()
    } catch (error) {
      if (result.status !== 'ready') {
        result.status = 'error'
        result.error = error instanceof Error ? error.message : String(error)
        notify()
      }
    }
  })()
  return result
}
