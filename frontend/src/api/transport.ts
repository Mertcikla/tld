import { createConnectTransport } from "@connectrpc/connect-web"
import { apiBase } from "../config/runtime"
import { collaborationIdentityHeaders } from "./collaborationIdentity"

export const transport = createConnectTransport({
  baseUrl: apiBase,
  fetch: (input, init) => {
    const headers = new Headers(init?.headers)
    for (const [key, value] of Object.entries(collaborationIdentityHeaders())) {
      headers.set(key, value)
    }
    return fetch(input, { ...init, headers, credentials: init?.credentials ?? 'include' })
  },
})
