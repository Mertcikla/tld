/**
 * Demo entry point.
 * Overrides the real `api` singleton with localStorage-backed implementations
 * and patches window.history to redirect /views/:id → /demo/:id, all for the
 * lifetime of this component. Restores everything on unmount.
 * No auth required; served at /demo and /demo/:id routes.
 */

import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Box } from '@chakra-ui/react'
import { api } from '../api/client'
import ViewEditor from '../pages/ViewEditor'
import { HeaderProvider } from '../components/HeaderContext'
import { ThemeProvider } from '../context/ThemeContext'
import { demoApi, initDemoStore } from './store'
import { DEMO_VIEW_EDITOR_OPTIONS } from './viewEditor'

// ── Override helpers ──────────────────────────────────────────────────────────

type ApiOverride = Record<string, unknown>

/**
 * Swap api methods + patch window.history so any /views/:id navigation is
 * silently rewritten to /demo/:id before React Router processes it.
 * Returns a function that restores everything.
 */
function applyOverrides(): () => void {
  const originals: [obj: ApiOverride, key: string, original: unknown][] = []

  function swap(obj: ApiOverride, key: string, replacement: unknown) {
    originals.push([obj, key, obj[key]])
    obj[key] = replacement
  }

  // elements
  const realElements = api.elements as unknown as ApiOverride
  const demoElements = demoApi.elements as ApiOverride
  for (const key of ['list', 'get', 'create', 'update', 'delete', 'placements']) {
    swap(realElements, key, demoElements[key])
  }

  // workspace.elements
  const realWsElements = (api.workspace as unknown as ApiOverride).elements as ApiOverride
  const demoWsElements = demoApi.workspace.elements as ApiOverride
  for (const key of ['list', 'get', 'create', 'update', 'delete', 'placements']) {
    swap(realWsElements, key, demoWsElements[key])
  }

  // workspace.orgs.tagColors
  const realTagColors = ((api.workspace as unknown as ApiOverride).orgs as ApiOverride).tagColors as ApiOverride
  const demoTagColors = demoApi.workspace.orgs.tagColors as ApiOverride
  for (const key of ['list', 'set']) {
    swap(realTagColors, key, demoTagColors[key])
  }

  // workspace.views top-level methods + nested namespaces
  const realViews = (api.workspace as unknown as ApiOverride).views as ApiOverride
  const demoViews = demoApi.workspace.views as ApiOverride
  for (const key of ['list', 'get', 'content', 'tree', 'create', 'update', 'delete', 'thumbnail', 'rename', 'setLevel', 'reparent']) {
    swap(realViews, key, demoViews[key])
  }
  for (const ns of ['placements', 'layers', 'reactions', 'threads']) {
    const realNs = realViews[ns] as ApiOverride
    const demoNs = demoViews[ns] as ApiOverride
    for (const key of Object.keys(demoNs)) {
      swap(realNs, key, demoNs[key])
    }
  }

  // workspace.connectors
  const realConnectors = (api.workspace as unknown as ApiOverride).connectors as ApiOverride
  const demoConnectors = demoApi.workspace.connectors as ApiOverride
  for (const key of ['list', 'create', 'update', 'delete']) {
    swap(realConnectors, key, demoConnectors[key])
  }

  // explore.load (cross-branch graph snapshot)
  const realExplore = (api as unknown as ApiOverride).explore as ApiOverride
  swap(realExplore, 'load', demoApi.explore.load)

  // ── Patch window.history so /views/:id → /demo/:id before React Router sees it
  const origPush = window.history.pushState.bind(window.history)
  const origReplace = window.history.replaceState.bind(window.history)

  function rewriteUrl(url: string | URL | null | undefined): string | URL | null | undefined {
    if (typeof url !== 'string') return url
    return url.replace(/\/views\/(\d+)/, '/demo/$1')
  }

  window.history.pushState = (state, title, url) => origPush(state, title, rewriteUrl(url))
  window.history.replaceState = (state, title, url) => origReplace(state, title, rewriteUrl(url))

  return () => {
    for (const [obj, key, original] of originals) {
      obj[key] = original
    }
    window.history.pushState = origPush
    window.history.replaceState = origReplace
  }
}

// ── Inner component overrides applied before children mount ─────────────────

function DemoApp({
  revealProgress,
}: {
  revealProgress: number
}) {
  // useState initializer runs synchronously during the first render of this
  // component instance, before any child component mounts or any hook effect
  // runs. This guarantees api overrides are in place for ViewEditor's first fetch.
  const [restore] = useState<() => void>(() => {
    initDemoStore()
    return applyOverrides()
  })

  useEffect(() => restore, [restore])

  return (
    <ViewEditor
      demoOptions={{ ...DEMO_VIEW_EDITOR_OPTIONS, revealProgress }}
    />
  )
}

// ── DemoNavigator: redirects bare /demo to the first root view ────────────────

export function DemoNavigator() {
  const navigate = useNavigate()

  useEffect(() => {
    // Call demoApi directly no need to apply global overrides here, which
    // would otherwise be cleaned up after DemoApp mounts and clobber its patches.
    initDemoStore()
    demoApi.workspace.views.tree().then((tree) => {
      const root = tree.find((v) => v.parent_view_id === null)
      navigate(root ? `/demo/${root.id}` : '/demo/1', { replace: true })
    }).catch(() => navigate('/demo/1', { replace: true }))
  }, [navigate])

  return null
}

// ── DemoPage ──────────────────────────────────────────────────────────────────

export default function DemoPage() {
  const [revealProgress, setRevealProgress] = useState(() => (window.self === window.top ? 1 : 0))

  useEffect(() => {
    if (window.self === window.top) {
      setRevealProgress(1)
      return
    }

    const handleMessage = (event: MessageEvent) => {
      const data = event.data as { type?: unknown; progress?: unknown } | null
      if (!data || data.type !== 'tldiagram-demo-progress') return

      const nextProgress = Number(data.progress)
      if (!Number.isFinite(nextProgress)) return

      setRevealProgress(Math.max(0, Math.min(1, nextProgress)))
    }

    window.addEventListener('message', handleMessage)
    return () => window.removeEventListener('message', handleMessage)
  }, [])

  return (
    <ThemeProvider
      storagePrefix="diag:demo"
      defaultAccent="#63b3ed"
      defaultBackground="#0d121e"
      defaultElementColor="#2d3748"
    >
      <Box minH="100vh" bg="var(--bg-canvas)" style={{ '--editor-top-offset': '0px' } as React.CSSProperties}>
        <HeaderProvider>
          <DemoApp revealProgress={revealProgress} />
        </HeaderProvider>
      </Box>
    </ThemeProvider>
  )
}
