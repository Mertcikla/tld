/* eslint-disable react-refresh/only-export-components */
import { createContext, useContext, useState, useMemo, type ReactNode } from 'react'

export interface Experimental {
  watchEnabled: boolean
}

const EXPERIMENTAL_KEY = 'tld:experimental'

const DEFAULT_EXPERIMENTAL: Experimental = {
  watchEnabled: false,
}

interface ExperimentalContextValue {
  experimental: Experimental
  toggleExperimental: (key: keyof Experimental) => void
}

const ExperimentalContext = createContext<ExperimentalContextValue>({
  experimental: DEFAULT_EXPERIMENTAL,
  toggleExperimental: () => {},
})

export function ExperimentalProvider({ children }: { children: ReactNode }) {
  const [experimental, setExperimental] = useState<Experimental>(() => {
    try {
      const stored = localStorage.getItem(EXPERIMENTAL_KEY)
      if (stored) {
        const parsed = JSON.parse(stored) as Partial<Experimental>
        return { ...DEFAULT_EXPERIMENTAL, ...parsed }
      }
    } catch {
      // ignore
    }
    return DEFAULT_EXPERIMENTAL
  })

  const toggleExperimental = (key: keyof Experimental) => {
    setExperimental((prev) => {
      const next = { ...prev, [key]: !prev[key] }
      try {
        localStorage.setItem(EXPERIMENTAL_KEY, JSON.stringify(next))
      } catch {
        // ignore
      }
      return next
    })
  }

  const value = useMemo(() => ({ experimental, toggleExperimental }), [experimental])

  return (
    <ExperimentalContext.Provider value={value}>
      {children}
    </ExperimentalContext.Provider>
  )
}

export function useExperimental() {
  return useContext(ExperimentalContext)
}
