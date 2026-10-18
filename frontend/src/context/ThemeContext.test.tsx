import React from 'react'
import { act, create } from 'react-test-renderer'
import { beforeEach, describe, expect, it } from 'vitest'
import { initializeTheme, ThemeProvider, useAccentColor, useTheme } from './ThemeContext'
import { ACCENT_OPTIONS } from '../constants/colors'

type ThemeControls = ReturnType<typeof useTheme>
let cssProperties: Map<string, string>

function installBrowserStubs() {
  const values = new Map<string, string>()
  cssProperties = new Map<string, string>()
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
          setProperty: (key: string, value: string) => { cssProperties.set(key, value) },
        },
      },
    },
  })
}

describe('ThemeProvider', () => {
  beforeEach(() => {
    installBrowserStubs()
  })

  it.each(ACCENT_OPTIONS)('initializes readable action labels for the $name accent', ({ name, value }) => {
    localStorage.setItem('theme-test:accent-color', value)
    initializeTheme('theme-test')

    expect(cssProperties.get('--accent')).toBe(value)
    expect(cssProperties.get('--accent-foreground')).toBe(name === 'Black' ? '#ffffff' : '#000000')
  })

  it('updates action label contrast when switching between custom dark and light accents', () => {
    let controls: ThemeControls | undefined
    function Controls() {
      controls = useTheme()
      return null
    }
    let renderer: ReturnType<typeof create>
    act(() => {
      renderer = create(<ThemeProvider storagePrefix="theme-test" defaultAccent="#123456"><Controls /></ThemeProvider>)
    })
    expect(cssProperties.get('--accent-foreground')).toBe('#ffffff')

    act(() => { controls?.setAccent('#abcdef') })
    expect(cssProperties.get('--accent-foreground')).toBe('#000000')

    act(() => { controls?.setAccent('#123456') })
    expect(cssProperties.get('--accent-foreground')).toBe('#ffffff')
    act(() => { renderer.unmount() })
  })

  it('does not rerender accent consumers when unrelated theme colors change', () => {
    let controls: ThemeControls | null = null
    let accentRenders = 0
    let accentValue: ReturnType<typeof useAccentColor> | null = null

    function currentAccentValue() {
      if (!accentValue) {
        throw new Error('Accent consumer did not render')
      }
      return accentValue
    }

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

    const initialAccentValue = currentAccentValue()
    expect(accentRenders).toBe(1)

    act(() => {
      controls?.setBackground('#101820')
    })
    act(() => {
      controls?.setElementColor('#1f2937')
    })

    expect(accentRenders).toBe(1)
    expect(currentAccentValue()).toBe(initialAccentValue)

    act(() => {
      controls?.setAccent('#ff3366')
    })

    expect(accentRenders).toBe(2)
    expect(currentAccentValue().accent).toBe('#ff3366')
  })
})
