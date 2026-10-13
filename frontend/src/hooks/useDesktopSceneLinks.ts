import { useEffect, useState } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import { isWailsApp } from '../config/runtime'
import { toast } from '../utils/toast'
import { LOCAL_SCENE_PREFIX, MAX_SCENE_URL_LENGTH } from '../utils/sceneUrl'

/** Subscribe before draining: startup and subsequent OS links use one queue. */
export function useDesktopSceneLinks() {
  const navigate = useNavigate()
  const location = useLocation()
  const [ready, setReady] = useState(!isWailsApp)
  const [pendingRoute, setPendingRoute] = useState<string>()
  const currentRoute = location.pathname + location.hash
  useEffect(() => {
    if (pendingRoute === currentRoute) setPendingRoute(undefined)
  }, [pendingRoute, currentRoute])
  useEffect(() => {
    if (!isWailsApp) return
    const bridge = window.go?.main?.DesktopBridge
    if (!bridge) { setReady(true); return }
    let active = true
    let running = false
    let requested = false
    const drain = async () => {
      requested = true
      if (running) return
      running = true
      try {
        while (requested && active) {
          requested = false
          const links = await bridge.DrainSceneLinks()
          // Deliver an already-drained batch even during StrictMode's effect
          // cleanup: it has been removed from Go's queue and must not be lost.
          for (const link of links) {
            if (link.error) toast({ status: 'error', title: 'Could not open comparison', description: link.error })
            else if (/^\/compare#v1=[A-Za-z0-9_-]+$/.test(link.route) && LOCAL_SCENE_PREFIX.length + link.route.slice('/compare#v1='.length).length <= MAX_SCENE_URL_LENGTH) {
              setPendingRoute(link.route)
              navigate(link.route)
            }
          }
        }
      } catch {
        toast({ status: 'error', title: 'Could not receive desktop comparison links' })
      } finally {
        running = false
        if (active) setReady(true)
      }
    }
    const unsubscribe = window.runtime?.EventsOn?.('tld:scene-links', () => { void drain() })
    void drain()
    return () => { active = false; unsubscribe?.() }
  }, [navigate])
  // Router navigation can commit in a transition after the queue promise
  // resolves. Do not mount the workspace at the old path in the meantime.
  return ready && (!pendingRoute || pendingRoute === currentRoute)
}
