import { useCallback, useState } from 'react'
import type { SubDetail } from '../utils/subElements'

function readLayout(key: string, fallback: SubDetail): SubDetail {
  try {
    const value = localStorage.getItem(key)
    if (value === 'overview' || value === 'outline' || value === 'code') return value
  } catch { /* Layout preferences are optional in embedded viewers. */ }
  return fallback
}

export function useSubElementLayout(viewId: number, elementId: number, fallback: SubDetail = 'code') {
  const key = `tld:element-layout:${viewId}:${elementId}`
  const [choice, setChoice] = useState(() => ({ key, detail: readLayout(key, fallback) }))
  const detail = choice.key === key ? choice.detail : readLayout(key, fallback)
  const setDetail = useCallback((next: SubDetail) => {
    setChoice({ key, detail: next })
    try { localStorage.setItem(key, next) } catch { /* Keep the current session usable. */ }
  }, [key])
  return { detail, setDetail }
}
