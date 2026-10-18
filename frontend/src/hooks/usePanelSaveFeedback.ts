import { useCallback, useRef, useState } from 'react'
import { useToast } from '@chakra-ui/react'

export type PanelSaveState = 'idle' | 'saving' | 'saved' | 'error'

/** Keep a completed request's feedback attached to the panel it came from. */
export function usePanelSaveFeedback(identity: string) {
  const [feedback, setFeedback] = useState<{ identity: string; state: PanelSaveState } | null>(null)
  const activeIdentity = useRef(identity)
  activeIdentity.current = identity
  const toast = useToast()
  const toastRef = useRef(toast)
  toastRef.current = toast

  const setState = useCallback((state: PanelSaveState) => {
    if (activeIdentity.current !== identity) return
    setFeedback((previous) => previous?.identity === identity && previous.state === state
      ? previous
      : { identity, state })
  }, [identity])

  const startSaving = useCallback(() => setState('saving'), [setState])
  const saved = useCallback(() => setState('saved'), [setState])
  const failed = useCallback(() => {
    setState('error')
    // Also report errors when a blur save finishes after its panel was closed.
    toastRef.current({
      id: `panel-save-error-${identity}`,
      title: 'Changes weren’t saved',
      description: 'Please try saving again.',
      status: 'error',
      duration: 6000,
      isClosable: true,
    })
  }, [identity, setState])

  return {
    state: feedback?.identity === identity ? feedback.state : 'idle' as PanelSaveState,
    startSaving,
    saved,
    failed,
  }
}
