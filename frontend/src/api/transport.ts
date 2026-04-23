import { createConnectTransport } from "@connectrpc/connect-web"
import { apiBase } from "../config/runtime"

export const transport = createConnectTransport({
  baseUrl: apiBase,
  fetch: (input, init) => fetch(input, init),
})
