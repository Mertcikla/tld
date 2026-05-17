import React from 'react'
import { act, create } from 'react-test-renderer'
import { beforeEach, describe, expect, it } from 'vitest'
import { ThemeProvider, useAccentColor, useTheme } from './ThemeContext'

type ThemeControls = ReturnType<typeof useTheme>

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
  Object.defineProperty(globalThis, 'document', {
    configurable: true,
    value: {
      documentElement: {
        style: {
          setProperty: () => { },
        },
      },
    },
  })
}

describe('ThemeProvider', () => {
  beforeEach(() => {
    installBrowserStubs()
  })

  it('does not rerender accent consumers when unrelated theme colors change', () => {
    let controls: ThemeControls | null = null
    let accentRenders = 0
    let accentValue: ReturnType<typeof useAccentColor> | null = null

    function Controls() {
      controls = useTheme()
      return null
    }

    function AccentConsumer() {
      accentRenders += 1
      accentValue = useAccentColor()
      return null
    }

    act(() => {
      create(
        <ThemeProvider storagePrefix="theme-test">
          <Controls />
          <AccentConsumer />
        </ThemeProvider>,
      )
    })

    const initialAccentValue = accentValue
    expect(accentRenders).toBe(1)

    act(() => {
      controls?.setBackground('#101820')
    })
    act(() => {
      controls?.setElementColor('#1f2937')
    })

    expect(accentRenders).toBe(1)
    expect(accentValue).toBe(initialAccentValue)

    act(() => {
      controls?.setAccent('#ff3366')
    })

    expect(accentRenders).toBe(2)
    expect(accentValue?.accent).toBe('#ff3366')
  })
})
