import { useEffect, useState } from 'react'
import type { LibraryElement } from '../types'
import { api } from '../api/client'

export interface ElementSearchResult {
  query: string
  setQuery: (q: string) => void
  remoteElements: LibraryElement[]
  fetching: boolean
  searchedQuery: string | null
}

export function useElementSearch(): ElementSearchResult {
  const [query, setQuery] = useState('')
  const [remoteElements, setRemoteElements] = useState<LibraryElement[]>([])
  const [fetching, setFetching] = useState(false)
  const [searchedQuery, setSearchedQuery] = useState<string | null>(null)

  useEffect(() => {
    const trimmed = query.trim()
    if (!trimmed) {
      setRemoteElements([])
      setFetching(false)
      setSearchedQuery(null)
      return
    }
    let cancelled = false
    setSearchedQuery(null)
    setFetching(true)
    const timer = setTimeout(() => {
      api.elements.list({ limit: 0, offset: 0, search: trimmed })
        .then((items) => {
          if (!cancelled) {
            setRemoteElements(items)
            setSearchedQuery(trimmed)
          }
        })
        .catch(() => {
          if (!cancelled) setRemoteElements([])
        })
        .finally(() => {
          if (!cancelled) setFetching(false)
        })
    }, 150)
    return () => {
      cancelled = true
      clearTimeout(timer)
    }
  }, [query])

  return { query, setQuery, remoteElements, fetching, searchedQuery }
}
