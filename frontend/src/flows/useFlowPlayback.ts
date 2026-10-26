import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { playbackFrames, type StoryFlow } from './model'

export function useFlowPlayback(flow: StoryFlow | null) {
  const [stepId, setStepId] = useState<string | null>(null)
  const [playing, setPlaying] = useState(false)
  const [seconds, setSeconds] = useState(3)
  const [loop, setLoop] = useState(false)
  const pause = useCallback(() => setPlaying(false), [])
  const frames = useMemo(() => playbackFrames(flow?.steps ?? []), [flow?.steps])
  const index = frames.findIndex(frame => frame.id === stepId)
  const frame = frames[index]

  const flowId = flow?.id
  const flowViewId = flow?.viewId
  const previousFlow = useRef<{ id: string; viewId: number } | null>(null)
  useEffect(() => {
    const previous = previousFlow.current
    previousFlow.current = flowId !== undefined && flowViewId !== undefined ? { id: flowId, viewId: flowViewId } : null
    setPlaying(false)
    // A newly saved draft is the same story, with a server-assigned identity.
    if (previous?.id === '' && flowId && previous.viewId === flowViewId) return
    setStepId(null)
  }, [flowId, flowViewId])
  useEffect(() => {
    if (stepId && index < 0) { setPlaying(false); setStepId(null) }
  }, [index, stepId])
  useEffect(() => {
    if (!playing || frames.length === 0) return
    const timer = setTimeout(() => {
      if (index + 1 < frames.length) setStepId(frames[index + 1].id)
      else if (loop) setStepId(frames[0].id)
      else setPlaying(false)
    }, seconds * 1000)
    return () => clearTimeout(timer)
  }, [playing, frames, index, seconds, loop])

  const select = useCallback((id: string | null) => { setPlaying(false); setStepId(id) }, [])
  function toggle() {
    if (playing) { setPlaying(false); return }
    if (!frames.length) return
    if (index < 0 || index === frames.length - 1) setStepId(frames[0].id)
    setPlaying(true)
  }
  return {
    frames, frame, index, playing, seconds, setSeconds, loop, setLoop, select, toggle,
    pause,
    next: () => select(frames[Math.min(frames.length - 1, index + 1)]?.id ?? null),
    previous: () => select(frames[index - 1]?.id ?? null),
  }
}
