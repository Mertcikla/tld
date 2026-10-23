import { useCallback, useRef, useState } from 'react'

type Fields = Record<string, string>
type Receipt = { value: string; revision: number; sequence: number }
type Session = { identity: string; draft: Fields; persisted: Fields; revisions: Record<string, number>; sequence: number }

/** A confirmation belongs to the exact draft and panel session that was saved. */
export function useAutosaveFieldFeedback(identity: string, draft: Fields, persisted: Fields) {
  const active = useRef<Session | null>(null)
  if (active.current?.identity !== identity) {
    active.current = { identity, draft: { ...draft }, persisted: { ...persisted }, revisions: {}, sequence: 0 }
  }
  const session = active.current
  for (const field of Object.keys(draft)) {
    if (session.draft[field] !== draft[field]) {
      session.revisions[field] = (session.revisions[field] ?? 0) + 1
    }
  }
  session.draft = { ...draft }

  const [feedback, setFeedback] = useState<{ session: Session; receipts: Record<string, Receipt>; pending: Record<string, Receipt>; inlineSave: boolean } | null>(null)

  // Capture before the request, confirm only after persistence succeeds.
  const capture = useCallback((submitted: Fields) => {
    const receipts: Record<string, Receipt> = {}
    for (const field of Object.keys(submitted)) {
      if (submitted[field] !== session.persisted[field]) {
        receipts[field] = {
          value: submitted[field],
          revision: session.draft[field] === submitted[field] ? session.revisions[field] ?? 0 : -1,
          sequence: ++session.sequence,
        }
      }
    }
    if (active.current === session) {
      setFeedback((previous) => ({
        session,
        receipts: previous?.session === session ? previous.receipts : {},
        pending: { ...(previous?.session === session ? previous.pending : {}), ...receipts },
        inlineSave: Object.keys(receipts).length > 0,
      }))
    }
    const finish = (succeeded: boolean) => {
      if (active.current !== session) return
      if (succeeded) session.persisted = { ...session.persisted, ...submitted }
      setFeedback((previous) => {
        const pending = { ...(previous?.session === session ? previous.pending : {}) }
        for (const [field, receipt] of Object.entries(receipts)) {
          if (pending[field]?.sequence === receipt.sequence) delete pending[field]
        }
        return {
          session,
          receipts: { ...(previous?.session === session ? previous.receipts : {}), ...(succeeded ? receipts : {}) },
          pending,
          inlineSave: Object.keys(receipts).length > 0,
        }
      })
    }
    return { saved: () => finish(true), failed: () => finish(false) }
  }, [session])

  const confirmed: Record<string, number | undefined> = {}
  if (feedback?.session === session) {
    for (const [field, receipt] of Object.entries(feedback.receipts)) {
      if (draft[field] === receipt.value && (session.revisions[field] ?? 0) === receipt.revision) {
        confirmed[field] = receipt.sequence
      }
    }
  }
  const saving: Record<string, boolean> = {}
  if (feedback?.session === session) {
    for (const field of Object.keys(feedback.pending)) saving[field] = true
  }
  return { capture, confirmed, saving, inlineSave: feedback?.session === session && feedback.inlineSave }
}
