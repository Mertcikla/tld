/* eslint-disable react-refresh/only-export-components */
import { createContext, useContext, useMemo, useState, type ReactNode } from 'react'

const DEFAULT_CONNECTOR_STYLE_KEY = 'diag:connector-style'

export const CONNECTOR_STYLE_OPTIONS = ['bezier', 'straight', 'step', 'smoothstep'] as const

export type ConnectorRouteStyle = (typeof CONNECTOR_STYLE_OPTIONS)[number]

export interface ConnectorStyleContextValue {
  defaultConnectorStyle: ConnectorRouteStyle | null
  setDefaultConnectorStyle: (style: ConnectorRouteStyle | null) => void
}

const ConnectorStyleContext = createContext<ConnectorStyleContextValue>({
  defaultConnectorStyle: null,
  setDefaultConnectorStyle: () => {},
})

function parseConnectorStyle(value: string | null): ConnectorRouteStyle | null {
  return CONNECTOR_STYLE_OPTIONS.includes(value as ConnectorRouteStyle) ? value as ConnectorRouteStyle : null
}

export function ConnectorStyleProvider({ children }: { children: ReactNode }) {
  const [defaultConnectorStyle, setDefaultConnectorStyleState] = useState<ConnectorRouteStyle | null>(() => {
    const stored = localStorage.getItem(DEFAULT_CONNECTOR_STYLE_KEY)
    return parseConnectorStyle(stored)
  })

  const setDefaultConnectorStyle = (style: ConnectorRouteStyle | null) => {
    setDefaultConnectorStyleState(style)
    if (style === null) {
      localStorage.removeItem(DEFAULT_CONNECTOR_STYLE_KEY)
      return
    }
    localStorage.setItem(DEFAULT_CONNECTOR_STYLE_KEY, style)
  }

  const value = useMemo(() => ({
    defaultConnectorStyle,
    setDefaultConnectorStyle,
  }), [defaultConnectorStyle])

  return (
    <ConnectorStyleContext.Provider value={value}>
      {children}
    </ConnectorStyleContext.Provider>
  )
}

export function useConnectorStyle() {
  return useContext(ConnectorStyleContext)
}

export function connectorStyleForCreate(defaultConnectorStyle: ConnectorRouteStyle | null) {
  return defaultConnectorStyle ?? undefined
}

export function connectorStyleForPreview(defaultConnectorStyle: ConnectorRouteStyle | null) {
  return defaultConnectorStyle ?? 'bezier'
}
