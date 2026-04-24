import { useEffect, useState } from 'react'
import { Routes, Route, Navigate, Outlet } from 'react-router-dom'
import { Box, Spinner, Center } from '@chakra-ui/react'
import { api } from './api/client'
import ViewEditor from './pages/ViewEditor'
import ViewsPage from './pages/Views'
import Dependencies from './pages/Dependencies'
import { SharedInfiniteZoom } from './pages/InfiniteZoom'
import Settings from './pages/Settings'
import AppearanceSettings from './pages/AppearanceSettings'
import { HeaderProvider, useHeader } from './components/HeaderContext'
import TopMenuBar from './components/TopMenuBar'
import { ThemeProvider } from './context/ThemeContext'
import { ACCENT_DEFAULT, BACKGROUND_DEFAULT, ELEMENT_DEFAULT, hexToRgba } from './constants/colors'
import { platform } from './platform/local'

function AppLayout() {
  const header = useHeader()
  const node = header && typeof header === 'object' && 'node' in header ? (header as { node: React.ReactNode }).node : header
  const hideMobileBar = header && typeof header === 'object' && 'hideMobileBar' in header ? !!(header as { hideMobileBar?: boolean }).hideMobileBar : false

  return (
    <Box h="100vh" display="flex" flexDirection="column" bg="var(--bg-canvas)" overflow="hidden">
      <TopMenuBar hideMobileBar={hideMobileBar}>
        {node}
      </TopMenuBar>
      <Box
        h={{ base: 'var(--topbar-h-mobile-total)', sm: 'var(--topbar-h-total)' }}
        mb={{ base: 'var(--topbar-content-gap)', sm: '0px' }}
        flexShrink={0}
      />
      <Box flex="1" overflow="hidden" position="relative">
        <Outlet />
      </Box>
    </Box>
  )
}

;(() => {
  const accent = localStorage.getItem('diag:accent-color') ?? ACCENT_DEFAULT
  document.documentElement.style.setProperty('--accent', accent)
  const rgba = hexToRgba(accent, 1)
  document.documentElement.style.setProperty('--accent-rgb', rgba.slice(5, -3))

  const background = localStorage.getItem('diag:background-color') ?? BACKGROUND_DEFAULT
  document.documentElement.style.setProperty('--bg-main', background)
  const bgRgba = hexToRgba(background, 1)
  document.documentElement.style.setProperty('--bg-main-rgb', bgRgba.slice(5, -3))

  const elementColor = localStorage.getItem('diag:element-color') ?? ELEMENT_DEFAULT
  document.documentElement.style.setProperty('--bg-element', elementColor)
  const objRgba = hexToRgba(elementColor, 1)
  document.documentElement.style.setProperty('--bg-element-rgb', objRgba.slice(5, -3))
})()

function HomeRedirect() {
  const [loading, setLoading] = useState(true)
  const [target, setTarget] = useState<string | null>(null)

  useEffect(() => {
    let mounted = true
    api.workspace.views
      .tree()
      .then((tree) => {
        if (!mounted) return
        const roots = (tree || []).filter((view) => view.parent_view_id === null)
        if (roots.length > 0) setTarget(`/views/${roots[0].id}`)
        else setTarget('/views')
      })
      .catch(() => mounted && setTarget('/views'))
      .finally(() => mounted && setLoading(false))

    return () => {
      mounted = false
    }
  }, [])

  if (loading) {
    return (
      <Center h="100vh">
        <Spinner size="xl" />
      </Center>
    )
  }

  return <Navigate to={target || '/views'} replace />
}

export default function App() {
  const [ready, setReady] = useState(false)

  useEffect(() => {
    api.system.ready()
      .then(() => platform.initPlatform())
      .finally(() => setReady(true))
  }, [])

  if (!ready) {
    return (
      <Center h="100vh">
        <Spinner size="xl" />
      </Center>
    )
  }

  return (
    <ThemeProvider>
      <Box minH="100vh" bg="var(--bg-canvas)">
        <Routes>
          {platform.getRoutes({ user: null })}

          <Route path="/explore/shared/:token" element={<Box h="100vh" overflow="hidden"><HeaderProvider><SharedInfiniteZoom /></HeaderProvider></Box>} />
          <Route
            element={
              <HeaderProvider>
                <AppLayout />
              </HeaderProvider>
            }
          >
            <Route index element={<HomeRedirect />} />
            <Route path="views" element={<ViewsPage />} />
            <Route path="views/:id" element={<ViewEditor />} />
            <Route path="dependencies" element={<Dependencies />} />
            <Route path="explore" element={<Navigate to="/views" replace />} />
            <Route path="settings" element={<Settings />}>
              <Route index element={<Navigate to="appearance" replace />} />
              {platform.getSettingsRoutes({ user: null })}
              <Route path="appearance" element={<AppearanceSettings />} />
            </Route>
          </Route>

          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </Box>
    </ThemeProvider>
  )
}
