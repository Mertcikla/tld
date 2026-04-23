import type { ExtensionToWebviewMessage, WebviewToExtensionMessage } from '../types/vscode-messages'

declare global {
  interface Window {
    __TLD_VSCODE__?: boolean
    __TLD_VSCODE_API__?: {
      postMessage: (msg: unknown) => void
    }
  }
}

const api = (window as Window).__TLD_VSCODE_API__

export const vscodeBridge = {
  postMessage: (msg: WebviewToExtensionMessage) => {
    api?.postMessage(msg)
  },
  onMessage: (handler: (msg: ExtensionToWebviewMessage) => void): (() => void) => {
    const listener = (e: MessageEvent) => {
      if (e.data && typeof e.data === 'object' && 'type' in e.data) {
        handler(e.data as ExtensionToWebviewMessage)
      }
    }
    window.addEventListener('message', listener)
    return () => window.removeEventListener('message', listener)
  },
}
