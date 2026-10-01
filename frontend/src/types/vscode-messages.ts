import type { LibraryElement } from './index'
import type { ViewMarkdownDocument } from './index'

export interface WorkspaceSymbol {
  name: string
  kind: string
  filePath: string
  startLine: number
}

// Messages sent from the VS Code extension host to the webview
export type ExtensionToWebviewMessage =
  | { type: 'workspace-symbols'; requestId: string; symbols: WorkspaceSymbol[] }
  | { type: 'workspace-files'; requestId: string; files: string[] }
  | { type: 'element-placed'; elementId: number; x: number; y: number }
  | { type: 'focus-element'; elementId: number }
  | { type: 'diagnostics-update'; elementId: number; severity: string; message: string }
  | { type: 'file-content'; requestId: string; content: string; startLineOffset: number }
  | { type: 'markdown-saved'; viewId: number; markdown: ViewMarkdownDocument; content: string }

// Messages sent from the webview to the VS Code extension host
export type WebviewToExtensionMessage =
  | { type: 'ready' }
  | {
      type: 'open-file'
      filePath: string
      startLine?: number
      symbolName?: string
      symbolKind?: string
      viewColumn?: 'active' | 'beside'
    }
  | {
      type: 'open-markdown'
      viewId: number
      path: string
      content?: string
      viewColumn?: 'active' | 'beside'
    }
  | { type: 'request-workspace-files'; requestId: string; pattern: string }
  | { type: 'request-symbol-list-for-file'; requestId: string; filePath: string }
  | { type: 'diagram-loaded'; diagramId: number; elements: LibraryElement[] }
  | { type: 'request-file-content'; requestId: string; filePath: string; startLine: number }

// Sync status payload
export interface SyncStatusPayload {
  localChanges: number
  needsPush: boolean
  needsPull: boolean
}
