import React from 'react'
import { act, create } from 'react-test-renderer'
import { beforeEach, describe, expect, it } from 'vitest'
import { ExperimentalProvider, useExperimental } from './ExperimentalContext'

type ExperimentalControls = ReturnType<typeof useExperimental>

function installBrowserStubs() {
  const values = new Map<string, string>()
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
      removeItem: (key: string) => values.delete(key),
      clear: () => values.clear(),
    },
  })
}

describe('ExperimentalProvider', () => {
  beforeEach(() => {
    installBrowserStubs()
  })

  it('provides default experimental features', () => {
    let controls: ExperimentalControls | null = null

    function Consumer() {
      controls = useExperimental()
      return null
    }

    act(() => {
      create(
        <ExperimentalProvider>
          <Consumer />
        </ExperimentalProvider>,
      )
    })

    expect(controls).not.toBeNull()
    expect(controls!.experimental.watchEnabled).toBe(false)
  })

  it('toggles and persists experimental features', () => {
    let controls: ExperimentalControls | null = null

    function Consumer() {
      controls = useExperimental()
      return null
    }

    act(() => {
      create(
        <ExperimentalProvider>
          <Consumer />
        </ExperimentalProvider>,
      )
    })

    expect(controls!.experimental.watchEnabled).toBe(false)

    act(() => {
      controls!.toggleExperimental('watchEnabled')
    })

    expect(controls!.experimental.watchEnabled).toBe(true)
    expect(globalThis.localStorage.getItem('tld:experimental')).toBe(
      JSON.stringify({ watchEnabled: true }),
    )

    act(() => {
      controls!.toggleExperimental('watchEnabled')
    })

    expect(controls!.experimental.watchEnabled).toBe(false)
    expect(globalThis.localStorage.getItem('tld:experimental')).toBe(
      JSON.stringify({ watchEnabled: false }),
    )
  })

  it('loads experimental features from localStorage on init', () => {
    globalThis.localStorage.setItem('tld:experimental', JSON.stringify({ watchEnabled: true }))

    let controls: ExperimentalControls | null = null

    function Consumer() {
      controls = useExperimental()
      return null
    }

    act(() => {
      create(
        <ExperimentalProvider>
          <Consumer />
        </ExperimentalProvider>,
      )
    })

    expect(controls!.experimental.watchEnabled).toBe(true)
  })
})
