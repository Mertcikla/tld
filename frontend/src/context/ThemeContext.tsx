/* eslint-disable react-refresh/only-export-components */
import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'
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

const ThemeContext = createContext<ThemeContextValue>({
  accent: ACCENT_DEFAULT,
  setAccent: () => { },
  background: BACKGROUND_DEFAULT,
  setBackground: () => { },
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

  function setAccent(value: string) {
    localStorage.setItem(accentKey, value)
    setAccentState(value)
    if (!storagePrefix) {
      api.user.updatePreferences({ accent_color: value }).catch(() => { })
    }
  }

  function setBackground(value: string) {
    localStorage.setItem(bgKey, value)
    setBackgroundState(value)
    if (!storagePrefix) {
      api.user.updatePreferences({ background_color: value }).catch(() => { })
    }
  }

  function setElementColor(value: string) {
    localStorage.setItem(elementKey, value)
    setElementColorState(value)
    if (!storagePrefix) {
      api.user.updatePreferences({ element_color: value }).catch(() => { })
    }
  }

  return (
    <ThemeContext.Provider value={{ accent, setAccent, background, setBackground, elementColor, setElementColor }}>
      {children}
    </ThemeContext.Provider>
  )
}

export function useTheme() {
  return useContext(ThemeContext)
}

/**
 * Backward compatibility alias
 * @deprecated Use useTheme() instead
 */
export function useAccentColor() {
  const { accent, setAccent } = useTheme()
  return { accent, setAccent }
}
