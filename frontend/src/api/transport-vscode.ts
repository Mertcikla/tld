/**
 * VS Code webview transport.
 *
 * Replaces the session-cookie transport with Bearer token auth.
 * The API key and server URL are injected by the extension host
 * via window globals before this script runs.
 */
import { createConnectTransport } from '@connectrpc/connect-web'

declare global {
  interface Window {
    __TLD_API_KEY__?: string
    __TLD_SERVER_URL__?: string
    __TLD_DIAGRAM_ID__?: number
  }
}

const serverUrl = (window.__TLD_SERVER_URL__ ?? 'https://tldiagram.com').replace(/\/$/, '')
const apiKey = window.__TLD_API_KEY__ ?? ''

export const transport = createConnectTransport({
  baseUrl: `${serverUrl}/api`,
  fetch: (input, init) => {
    const headers = new Headers(init?.headers)
    if (apiKey) headers.set('Authorization', `Bearer ${apiKey}`)
    return fetch(input, { ...init, headers })
  },
})
