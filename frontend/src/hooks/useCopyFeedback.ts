import { useEffect, useRef, useState } from 'react'

// Existing hosts may supply synchronous callbacks. Return false when a handled
// error or empty export should suppress success feedback.
export type CopyAction = () => void | boolean | Promise<void | boolean>

export function useCopyFeedback(action?: CopyAction) {
  const [state, setState] = useState<'idle' | 'copying' | 'copied'>('idle')
  const pending = useRef(false)
  const mounted = useRef(true)
  const resetTimer = useRef<ReturnType<typeof setTimeout>>()

  useEffect(() => {
    mounted.current = true
    return () => {
      mounted.current = false
      clearTimeout(resetTimer.current)
    }
  }, [])

  const copy = async () => {
    if (!action || pending.current) return
    pending.current = true
    clearTimeout(resetTimer.current)
    setState('copying')
    try {
      const result = await action()
      if (!mounted.current) return
      setState(result === false ? 'idle' : 'copied')
      if (result !== false) {
        resetTimer.current = setTimeout(() => setState('idle'), 1800)
      }
    } catch {
      // The action owns error reporting; a rejected copy must never show success.
      if (mounted.current) setState('idle')
    } finally {
      pending.current = false
    }
  }

  return { state, copy }
}
