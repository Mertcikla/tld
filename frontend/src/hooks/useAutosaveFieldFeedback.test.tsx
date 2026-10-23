import { act, create, type ReactTestRenderer } from 'react-test-renderer'
import { afterEach, describe, expect, it } from 'vitest'
import { useAutosaveFieldFeedback } from './useAutosaveFieldFeedback'

const original = { name: 'API', description: '' }
let renderer: ReactTestRenderer
let feedback: ReturnType<typeof useAutosaveFieldFeedback>
function Harness({ identity = 'element-1', draft = original }: { identity?: string; draft?: typeof original }) {
  feedback = useAutosaveFieldFeedback(identity, draft, original)
  return null
}
afterEach(() => act(() => renderer?.unmount()))

describe('autosaved field confirmations', () => {
  it('confirms only changed fields after persistence and invalidates them immediately on editing', () => {
    act(() => { renderer = create(<Harness />) })
    expect(feedback.confirmed).toEqual({})
    const draft = { ...original, name: 'Payments' }
    act(() => renderer.update(<Harness draft={draft} />))
    let confirm!: ReturnType<typeof feedback.capture>
    act(() => { confirm = feedback.capture(draft) })
    expect(feedback.confirmed).toEqual({})
    expect(feedback.saving).toEqual({ name: true })
    act(() => confirm.saved())
    expect(feedback.saving).toEqual({})
    expect(feedback.confirmed.name).toBeDefined()
    expect(feedback.confirmed.description).toBeUndefined()
    expect(feedback.inlineSave).toBe(true)
    act(() => renderer.update(<Harness draft={{ ...draft, name: 'Payments v2' }} />))
    expect(feedback.confirmed.name).toBeUndefined()
    // Returning to an earlier value must not resurrect old feedback.
    act(() => renderer.update(<Harness draft={draft} />))
    expect(feedback.confirmed.name).toBeUndefined()
  })

  it('does not confirm newer edits when an older request finishes', () => {
    const draft = { ...original, name: 'First edit' }
    act(() => { renderer = create(<Harness draft={draft} />) })
    let confirm!: ReturnType<typeof feedback.capture>
    act(() => { confirm = feedback.capture(draft) })
    act(() => renderer.update(<Harness draft={{ ...draft, name: 'Second edit' }} />))
    act(() => confirm.saved())
    expect(feedback.confirmed.name).toBeUndefined()
    let confirmLatest!: ReturnType<typeof feedback.capture>
    act(() => { confirmLatest = feedback.capture({ ...draft, name: 'Second edit' }) })
    act(() => confirmLatest.saved())
    expect(feedback.confirmed.name).toBeDefined()
  })

  it('keeps late results out of a different or reopened panel', () => {
    act(() => { renderer = create(<Harness draft={{ ...original, name: 'Edited' }} />) })
    let confirm!: ReturnType<typeof feedback.capture>
    act(() => { confirm = feedback.capture({ ...original, name: 'Edited' }) })
    act(() => renderer.update(<Harness identity="closed" />))
    act(() => renderer.update(<Harness />))
    act(() => confirm.saved())
    expect(feedback.confirmed).toEqual({})
    expect(feedback.inlineSave).toBe(false)
  })

  it('leaves unrelated control saves to the panel feedback', () => {
    act(() => { renderer = create(<Harness />) })
    act(() => feedback.capture(original).saved())
    expect(feedback.confirmed).toEqual({})
    expect(feedback.inlineSave).toBe(false)
  })

  it('stops pending feedback on failure without confirming the draft and supports a retry', () => {
    const draft = { ...original, name: 'Payments' }
    act(() => { renderer = create(<Harness draft={draft} />) })
    let request!: ReturnType<typeof feedback.capture>
    act(() => { request = feedback.capture(draft) })
    expect(feedback.saving.name).toBe(true)
    act(() => request.failed())
    expect(feedback.saving).toEqual({})
    expect(feedback.confirmed).toEqual({})
    act(() => { request = feedback.capture(draft) })
    expect(feedback.saving.name).toBe(true)
    act(() => request.saved())
    expect(feedback.saving).toEqual({})
    expect(feedback.confirmed.name).toBeDefined()
  })

  it('keeps a newer request pending when an older request finishes', () => {
    const draft = { ...original, name: 'First edit' }
    act(() => { renderer = create(<Harness draft={draft} />) })
    let first!: ReturnType<typeof feedback.capture>
    let second!: ReturnType<typeof feedback.capture>
    act(() => { first = feedback.capture(draft) })
    const newer = { ...draft, name: 'Second edit' }
    act(() => renderer.update(<Harness draft={newer} />))
    act(() => { second = feedback.capture(newer) })
    act(() => first.saved())
    expect(feedback.saving.name).toBe(true)
    expect(feedback.confirmed.name).toBeUndefined()
    act(() => second.saved())
    expect(feedback.saving).toEqual({})
    expect(feedback.confirmed.name).toBeDefined()
  })
})
