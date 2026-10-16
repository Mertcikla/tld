import { useEffect, useRef, useState } from 'react'
import { api, type RepositoryGitHistory } from '../api/client'

export function useRepositoryHistorySearch(repositoryId: string, branch: string, revision: string, query: string, author: string, tag: string) {
  const term = query.trim()
  const active = !!(term || author || tag)
  const key = JSON.stringify([repositoryId, branch, revision, term, author, tag])
  const [result, setResult] = useState<{ key: string; history: RepositoryGitHistory } | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [retry, setRetry] = useState(0)
  const request = useRef<AbortController | null>(null)
  useEffect(() => {
    setResult(null)
    setError('')
    setLoading(active)
    if (!active || !repositoryId) return
    const controller = new AbortController()
    request.current = controller
    const timer = setTimeout(() => {
      void api.repositories.history(repositoryId, branch, 100, { query: term, author, tag, signal: controller.signal }).then((history) => {
        if (!controller.signal.aborted) setResult({ key, history })
      }).catch((err: unknown) => {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'Could not search commits')
      }).finally(() => {
        if (!controller.signal.aborted) {
          request.current = null
          setLoading(false)
        }
      })
    }, 300)
    return () => {
      clearTimeout(timer)
      controller.abort()
      request.current?.abort()
      request.current = null
    }
  }, [repositoryId, branch, term, author, tag, active, key, retry])
  const history = result?.key === key ? result.history : null
  const loadMore = async () => {
    if (!history?.nextPageToken || request.current) return
    const controller = new AbortController()
    request.current = controller
    setLoading(true)
    setError('')
    try {
      const next = await api.repositories.history(repositoryId, branch, 100, { query: term, author, tag, pageToken: history.nextPageToken, signal: controller.signal })
      if (!controller.signal.aborted) setResult({ key, history: { ...history, commits: [...history.commits, ...next.commits], hasMore: next.hasMore, nextPageToken: next.nextPageToken } })
    } catch (err) {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : 'Could not load matching commits')
    } finally {
      if (request.current === controller) {
        request.current = null
        setLoading(false)
      }
    }
  }
  return { active, history, loading, error, loadMore, retry: () => setRetry((value) => value + 1) }
}
