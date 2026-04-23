import type { ExtensionToWebviewMessage, WebviewToExtensionMessage } from '../types/vscode-messages'

// No-op stub used in web/native builds. Swapped for vscodeBridge-vscode.ts in VS Code builds.
export const vscodeBridge = {
  postMessage: (_msg: WebviewToExtensionMessage) => {},
  onMessage: (_handler: (msg: ExtensionToWebviewMessage) => void): (() => void) => () => {},
}
