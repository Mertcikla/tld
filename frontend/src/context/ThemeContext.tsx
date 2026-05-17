/* eslint-disable react-refresh/only-export-components */
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react'
import { ACCENT_DEFAULT, BACKGROUND_DEFAULT, ELEMENT_DEFAULT, hexToRgba } from '../constants/colors'
import { api } from '../api/client'

const ACCENT_KEY = 'diag:accent-color'
const BG_KEY = 'diag:background-color'
const ELEMENT_COLOR_KEY = 'diag:element-color'

/** Convert hex to "r,g,b" triplet for use in rgba(var(--rgb), alpha). */
function toRgbTriplet(hex: string): string {
  const rgba = hexToRgba(hex, 1)
  // hexToRgba returns "rgba(r,g,b,1)" - extract "r,g,b"
  return rgba.slice(5, -3)
}

function applyAccentVars(hex: string) {
  document.documentElement.style.setProperty('--accent', hex)
  document.documentElement.style.setProperty('--accent-rgb', toRgbTriplet(hex))
}

function applyBgVars(hex: string) {
  document.documentElement.style.setProperty('--bg-main', hex)
  document.documentElement.style.setProperty('--bg-main-rgb', toRgbTriplet(hex))

  // Also derive canvas background (slightly darker)
  // For simplicity, we just use the same or a hardcoded variant for now,
  // but we could use color-mix if we wanted true derivation.
  // document.documentElement.style.setProperty('--bg-canvas', `color-mix(in srgb, ${hex}, black 20%)`)
}

function applyElementVars(hex: string) {
  document.documentElement.style.setProperty('--bg-element', hex)
  document.documentElement.style.setProperty('--bg-element-rgb', toRgbTriplet(hex))
}

interface ThemeContextValue {
  accent: string
  setAccent: (value: string) => void
  background: string
  setBackground: (value: string) => void
  elementColor: string
  setElementColor: (value: string) => void
}

interface AccentColorContextValue {
  accent: string
  setAccent: (value: string) => void
}

interface BackgroundColorContextValue {
  background: string
  setBackground: (value: string) => void
}

interface ElementColorContextValue {
  elementColor: string
  setElementColor: (value: string) => void
}

const AccentColorContext = createContext<AccentColorContextValue>({
  accent: ACCENT_DEFAULT,
  setAccent: () => { },
})

const BackgroundColorContext = createContext<BackgroundColorContextValue>({
  background: BACKGROUND_DEFAULT,
  setBackground: () => { },
})

const ElementColorContext = createContext<ElementColorContextValue>({
  elementColor: ELEMENT_DEFAULT,
  setElementColor: () => { },
})

export function ThemeProvider({
  children,
  isAuthenticated,
  defaultAccent,
  defaultBackground,
  defaultElementColor,
  storagePrefix,
}: {
  children: ReactNode
  isAuthenticated?: boolean
  defaultAccent?: string
  defaultBackground?: string
  defaultElementColor?: string
  storagePrefix?: string
}) {
  const accentKey = storagePrefix ? `${storagePrefix}:accent-color` : ACCENT_KEY
  const bgKey = storagePrefix ? `${storagePrefix}:background-color` : BG_KEY
  const elementKey = storagePrefix ? `${storagePrefix}:element-color` : ELEMENT_COLOR_KEY

  const [accent, setAccentState] = useState<string>(
    () => localStorage.getItem(accentKey) ?? defaultAccent ?? ACCENT_DEFAULT,
  )
  const [background, setBackgroundState] = useState<string>(
    () => localStorage.getItem(bgKey) ?? defaultBackground ?? BACKGROUND_DEFAULT,
  )
  const [elementColor, setElementColorState] = useState<string>(
    () => localStorage.getItem(elementKey) ?? defaultElementColor ?? ELEMENT_DEFAULT,
  )

  // Apply CSS vars whenever accent or background changes
  useEffect(() => {
    applyAccentVars(accent)
  }, [accent])

  useEffect(() => {
    applyBgVars(background)
  }, [background])

  useEffect(() => {
    applyElementVars(elementColor)
  }, [elementColor])

  // Fetch server preferences only when authenticated and NOT in namespaced/demo mode
  useEffect(() => {
    if (!isAuthenticated || storagePrefix) return
    api.user.getPreferences().then((prefs) => {
      if (prefs.accent_color && prefs.accent_color !== accent) {
        localStorage.setItem(accentKey, prefs.accent_color)
        setAccentState(prefs.accent_color)
      }
      if (prefs.background_color && prefs.background_color !== background) {
        localStorage.setItem(bgKey, prefs.background_color)
        setBackgroundState(prefs.background_color)
      }
      if (prefs.element_color && prefs.element_color !== elementColor) {
        localStorage.setItem(elementKey, prefs.element_color)
        setElementColorState(prefs.element_color)
      }
    }).catch(() => { })
  }, [isAuthenticated, storagePrefix]) // eslint-disable-line react-hooks/exhaustive-deps

  const setAccent = useCallback((value: string) => {
    localStorage.setItem(accentKey, value)
    setAccentState(value)
    if (!storagePrefix) {
      api.user.updatePreferences({ accent_color: value }).catch(() => { })
    }
  }, [accentKey, storagePrefix])

  const setBackground = useCallback((value: string) => {
    localStorage.setItem(bgKey, value)
    setBackgroundState(value)
    if (!storagePrefix) {
      api.user.updatePreferences({ background_color: value }).catch(() => { })
    }
  }, [bgKey, storagePrefix])

  const setElementColor = useCallback((value: string) => {
    localStorage.setItem(elementKey, value)
    setElementColorState(value)
    if (!storagePrefix) {
      api.user.updatePreferences({ element_color: value }).catch(() => { })
    }
  }, [elementKey, storagePrefix])

  const accentValue = useMemo(() => ({ accent, setAccent }), [accent, setAccent])
  const backgroundValue = useMemo(() => ({ background, setBackground }), [background, setBackground])
  const elementColorValue = useMemo(() => ({ elementColor, setElementColor }), [elementColor, setElementColor])

  return (
    <AccentColorContext.Provider value={accentValue}>
      <BackgroundColorContext.Provider value={backgroundValue}>
        <ElementColorContext.Provider value={elementColorValue}>
          {children}
        </ElementColorContext.Provider>
      </BackgroundColorContext.Provider>
    </AccentColorContext.Provider>
  )
}

export function useTheme() {
  const { accent, setAccent } = useContext(AccentColorContext)
  const { background, setBackground } = useContext(BackgroundColorContext)
  const { elementColor, setElementColor } = useContext(ElementColorContext)

  return useMemo<ThemeContextValue>(() => ({
    accent,
    setAccent,
    background,
    setBackground,
    elementColor,
    setElementColor,
  }), [accent, setAccent, background, setBackground, elementColor, setElementColor])
}

/**
 * Backward compatibility alias
 * @deprecated Use useTheme() instead
 */
export function useAccentColor() {
  return useContext(AccentColorContext)
}
