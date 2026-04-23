/* eslint-disable react-refresh/only-export-components */
import React, { createContext, useContext, useState } from 'react'

type HeaderPayload = { node: React.ReactNode | null; hideMobileBar?: boolean } | React.ReactNode | null
type HeaderSetter = (payload: HeaderPayload) => void

const HeaderContext = createContext<{ header: HeaderPayload; setHeader: HeaderSetter } | undefined>(undefined)

export function HeaderProvider({ children }: { children: React.ReactNode }) {
  const [header, setHeader] = useState<HeaderPayload>(null)
  return (
    <HeaderContext.Provider value={{ header, setHeader }}>
      {children}
    </HeaderContext.Provider>
  )
}

export function useSetHeader() {
  const ctx = useContext(HeaderContext)
  if (!ctx) throw new Error('useSetHeader must be used within HeaderProvider')
  return ctx.setHeader
}

export function useHeader() {
  const ctx = useContext(HeaderContext)
  if (!ctx) throw new Error('useHeader must be used within HeaderProvider')
  return ctx.header
}

export default HeaderContext
