import { useEffect, useState } from 'react'
import { Routes, Route, Navigate, Outlet, useSearchParams } from 'react-router-dom'
import { Box, HStack, Spinner, Center } from '@chakra-ui/react'
import { api } from './api/client'
import ViewEditor from './pages/ViewEditor'
import ViewsPage from './pages/Views'
import Inventory from './pages/Inventory'
import { SharedInfiniteZoom } from './pages/InfiniteZoom'
import Settings from './pages/Settings'
import ProfileSettings from './pages/ProfileSettings'
import AppearanceSettings from './pages/AppearanceSettings'
import ExperimentalSettings from './pages/ExperimentalSettings'
import UpdateSettings from './pages/UpdateSettings'
import { HeaderProvider, useHeader } from './components/HeaderContext'
import TopMenuBar from './components/TopMenuBar'
import TopMenuBarCollaboration, { type CollaborationProps } from './components/TopMenuBarCollaboration'
import WorkspacePanel from './components/WorkspacePanel'
import { ExperimentalProvider, useExperimental } from './context/ExperimentalContext'
import { WorkspaceVersionProvider } from './context/WorkspaceVersionContext'
import { ConnectorStyleProvider } from './context/ConnectorStyleContext'
import { initializeTheme, ThemeProvider } from './context/ThemeContext'
import { platform } from './platform/local'
import { HomeRedirect } from './components/HomeRedirect'
import { isWailsApp, isWailsAppStore, isWailsWindows } from './config/runtime'

initializeTheme()

function AppLayout() {
  const header = useHeader()
  const node = header && typeof header === 'object' && 'node' in header ? (header as { node: React.ReactNode }).node : header
  const hideMobileBar = header && typeof header === 'object' && 'hideMobileBar' in header ? !!(header as { hideMobileBar?: boolean }).hideMobileBar : false
  const collaboration = header && typeof header === 'object' && 'collaboration' in header ? (header as { collaboration?: CollaborationProps }).collaboration : undefined
  const hideTopBar = typeof window !== 'undefined' && !!window.__TLD_VSCODE__
  const { experimental } = useExperimental()
  const rightSlot = collaboration || experimental.watchEnabled
    ? (
      <HStack spacing={2}>
        {collaboration && <TopMenuBarCollaboration collaboration={collaboration} />}
        {experimental.watchEnabled && <WorkspacePanel />}
      </HStack>
    )
    : undefined

  return (
    <Box
      h="var(--app-viewport-height)"
      display="flex"
      flexDirection="column"
      bg="var(--bg-canvas)"
      overflow="hidden"
      style={isWailsApp ? {
        "--topbar-h": "52px",
        "--topbar-h-total": "52px",
        "--wails-window-controls-w": isWailsWindows ? "138px" : "0px",
      } as React.CSSProperties : undefined}
    >
      {!hideTopBar && (
        <>
          <TopMenuBar hideMobileBar={hideMobileBar} rightSlot={rightSlot}>
            {node}
          </TopMenuBar>
          <Box
            h={{ base: 'var(--topbar-h-mobile-total)', sm: 'var(--topbar-h-total)' }}
            mb={{ base: 'var(--topbar-content-gap)', sm: '0px' }}
            flexShrink={0}
          />
        </>
      )}
      <Box
        flex="1"
        minH={0}
        overflow="hidden"
        position="relative"
        pb={{ base: hideTopBar ? 0 : 'calc(var(--bottomnav-h) + env(safe-area-inset-bottom, 0px))', sm: 0 }}
      >
        <Outlet />
      </Box>
    </Box>
  )
}

function DependenciesRedirect() {
  const [searchParams] = useSearchParams()
  const elementId = searchParams.get('element')
  const target = elementId ? `/inventory?object=element:${elementId}` : '/inventory'
  return <Navigate to={target} replace />
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
      <Center h="var(--app-viewport-height)">
        <Spinner size="xl" />
      </Center>
    )
  }

  return (
      <ExperimentalProvider>
      <ThemeProvider>
      <ConnectorStyleProvider>
        <Box h="var(--app-viewport-height)" bg="var(--bg-canvas)" overflow="hidden">
          <Routes>
            {platform.getRoutes({ user: null })}

            <Route path="/explore/shared/:token" element={<Box h="var(--app-viewport-height)" overflow="hidden"><HeaderProvider><WorkspaceVersionProvider><SharedInfiniteZoom /></WorkspaceVersionProvider></HeaderProvider></Box>} />
            <Route
              element={
                <HeaderProvider>
                  <WorkspaceVersionProvider>
                    <AppLayout />
                  </WorkspaceVersionProvider>
                </HeaderProvider>
              }
            >
              <Route index element={<HomeRedirect />} />
              <Route path="views" element={<ViewsPage />} />
              <Route path="views/:id" element={<ViewEditor />} />
              <Route path="inventory" element={<Inventory />} />
              <Route path="dependencies" element={<DependenciesRedirect />} />
              <Route path="explore" element={<Navigate to="/views" replace />} />
              <Route path="settings" element={<Settings />}>
                <Route index element={<Navigate to="profile" replace />} />
                {platform.getSettingsRoutes({ user: null })}
                <Route path="profile" element={<ProfileSettings />} />
                <Route path="appearance" element={<AppearanceSettings />} />
                <Route path="experimental" element={<ExperimentalSettings />} />
                <Route path="updates" element={isWailsApp && !isWailsAppStore ? <UpdateSettings /> : <Navigate to="/settings/appearance" replace />} />
              </Route>
            </Route>

            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Box>
      </ConnectorStyleProvider>
      </ThemeProvider>
    </ExperimentalProvider>
  )
}
