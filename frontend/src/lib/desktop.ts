import { isWailsApp } from '../config/runtime'

export interface DialogFilter {
  displayName: string
  pattern: string
}

export interface FileDialogResult {
  path: string
  content: string
  canceled: boolean
}

export interface SaveFileResult {
  path: string
  canceled: boolean
}

export interface DesktopUpdateStatus {
  checked: boolean
  current: string
  latest: string
  updateAvailable: boolean
  releaseUrl: string
  assetName: string
  cached: boolean
  supported: boolean
  canInstall: boolean
  installStarted: boolean
  restartRequired: boolean
  message?: string
}

type FileDropCallback = (x: number, y: number, paths: string[]) => void

interface DesktopBridge {
  SaveFile(defaultFilename: string, filters: DialogFilter[], base64Content: string): Promise<SaveFileResult>
  OpenTextFile(filters: DialogFilter[]): Promise<FileDialogResult>
  ReadTextFile(path: string): Promise<FileDialogResult>
  OpenPath(path: string): Promise<void>
  CheckForUpdate(): Promise<DesktopUpdateStatus>
  InstallUpdate(): Promise<DesktopUpdateStatus>
}

declare global {
  interface Window {
    go?: {
      main?: {
        DesktopBridge?: DesktopBridge
      }
    }
    runtime?: {
      BrowserOpenURL?: (url: string) => void
      OnFileDrop?: (callback: FileDropCallback, useDropTarget: boolean) => void
      OnFileDropOff?: () => void
    }
  }
}

export const mermaidImportFilters: DialogFilter[] = [
  { displayName: 'Diagram Files (*.md;*.dsl;*.txt)', pattern: '*.md;*.dsl;*.txt' },
  { displayName: 'All Files (*.*)', pattern: '*.*' },
]

function desktopBridge(): DesktopBridge {
  const bridge = window.go?.main?.DesktopBridge
  if (!bridge) throw new Error('Desktop bridge is not available')
  return bridge
}

export function openExternalUrl(url: string) {
  if (!url) return
  if (isWailsApp && window.runtime?.BrowserOpenURL) {
    window.runtime.BrowserOpenURL(url)
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}

export async function saveBlobAs(blob: Blob, filename: string, filters: DialogFilter[] = []): Promise<SaveFileResult> {
  if (isWailsApp) {
    const base64Content = await blobToBase64(blob)
    return desktopBridge().SaveFile(filename, filters, base64Content)
  }

  const href = URL.createObjectURL(blob)
  try {
    const link = document.createElement('a')
    link.href = href
    link.download = filename
    document.body.appendChild(link)
    link.click()
    document.body.removeChild(link)
  } finally {
    URL.revokeObjectURL(href)
  }
  return { path: '', canceled: false }
}

export async function saveDataUrlAs(dataUrl: string, filename: string, filters: DialogFilter[] = []): Promise<SaveFileResult> {
  return saveBlobAs(await dataUrlToBlob(dataUrl), filename, filters)
}

export async function openTextFile(filters: DialogFilter[] = mermaidImportFilters): Promise<FileDialogResult> {
  if (!isWailsApp) {
    throw new Error('Native file open is only available in the desktop app')
  }
  return desktopBridge().OpenTextFile(filters)
}

export async function readTextFile(path: string): Promise<FileDialogResult> {
  if (!isWailsApp) {
    throw new Error('Native file read is only available in the desktop app')
  }
  return desktopBridge().ReadTextFile(path)
}

export async function checkForDesktopUpdate(): Promise<DesktopUpdateStatus> {
  if (!isWailsApp) {
    throw new Error('Update checks are only available in the desktop app')
  }
  return desktopBridge().CheckForUpdate()
}

export async function installDesktopUpdate(): Promise<DesktopUpdateStatus> {
  if (!isWailsApp) {
    throw new Error('Self update is only available in the desktop app')
  }
  return desktopBridge().InstallUpdate()
}

export function onFileDrop(callback: FileDropCallback): (() => void) | null {
  if (!isWailsApp || !window.runtime?.OnFileDrop || !window.runtime?.OnFileDropOff) return null
  window.runtime.OnFileDrop(callback, false)
  return () => window.runtime?.OnFileDropOff?.()
}

export async function dataUrlToBlob(dataUrl: string): Promise<Blob> {
  const response = await fetch(dataUrl)
  return response.blob()
}

export async function blobToBase64(blob: Blob): Promise<string> {
  const bytes = new Uint8Array(await blob.arrayBuffer())
  let binary = ''
  const chunkSize = 0x8000
  for (let offset = 0; offset < bytes.length; offset += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(offset, offset + chunkSize))
  }
  return btoa(binary)
}
