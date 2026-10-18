import { useCallback, useRef, useState } from 'react'
import { useToast } from '@chakra-ui/react'

export type PanelSaveState = 'idle' | 'saving' | 'saved' | 'error'
type PanelSession = { identity: string }

/** Keep a completed request's feedback attached to the panel it came from. */
export function usePanelSaveFeedback(identity: string) {
  const [feedback, setFeedback] = useState<{ session: PanelSession; state: PanelSaveState } | null>(null)
  const activeSession = useRef<PanelSession>({ identity })
  // Reopening the same item starts a new draft session, including for late results.
  if (activeSession.current.identity !== identity) activeSession.current = { identity }
  const session = activeSession.current
  const toast = useToast()
  const toastRef = useRef(toast)
  toastRef.current = toast

  const setState = useCallback((state: PanelSaveState) => {
    if (activeSession.current !== session) return
    setFeedback((previous) => previous?.session === session && previous.state === state
      ? previous
      : { session, state })
  }, [session])

  const startSaving = useCallback(() => setState('saving'), [setState])
  const saved = useCallback(() => setState('saved'), [setState])
  const failed = useCallback(() => {
    setState('error')
    // The open panel owns its toast. Use a global notification only after leaving it.
    if (activeSession.current === session) return
    toastRef.current({
      id: `panel-save-error-${identity}`,
      title: 'Changes weren’t saved',
      description: 'Please try saving again.',
      status: 'error',
      duration: 6000,
      isClosable: true,
    })
  }, [identity, session, setState])

  return {
    state: feedback?.session === session ? feedback.state : 'idle' as PanelSaveState,
    startSaving,
    saved,
    failed,
  }
}
