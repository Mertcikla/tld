import { useContext, useEffect, useRef, useState } from 'react'
import { CheckCircleIcon, EditIcon, WarningIcon } from '@chakra-ui/icons'
import { NavigationIcon } from '../Icons'
import { MermaidMarkdownContext } from './mermaidContext'

type MermaidApi = typeof import('mermaid').default

type MermaidRenderState =
  | { status: 'empty'; svg: ''; error: null }
  | { status: 'loading'; svg: ''; error: null }
  | { status: 'ready'; svg: string; error: null }
  | { status: 'error'; svg: ''; error: string }

interface MermaidPreviewProps {
  code: string
}

let mermaidApiPromise: Promise<MermaidApi> | null = null
let mermaidRenderId = 0

const tldMarkerPattern = /^%%[ \t]+tld\/v1(?:[ \t]+(.*))?$/
const tldMarkerViewPattern = /(?:^|\s)(?:view|viewId)=([0-9]+)/

function resolveCssColor(value: string, fallback: string) {
  if (typeof document === 'undefined' || !document.body) return fallback

  const probe = document.createElement('span')
  probe.style.color = value || fallback
  probe.style.display = 'none'
  document.body.appendChild(probe)
  const resolved = getComputedStyle(probe).color
  probe.remove()

  return resolved || fallback
}

function themeColor(cssVariable: string, fallback: string) {
  if (typeof document === 'undefined') return fallback
  const value = getComputedStyle(document.documentElement).getPropertyValue(cssVariable).trim()
  return resolveCssColor(value, fallback)
}

function bodyTextColor() {
  if (typeof document === 'undefined' || !document.body) return 'CanvasText'
  return resolveCssColor(getComputedStyle(document.body).color, 'CanvasText')
}

function mermaidThemeVariables() {
  const elementBackground = themeColor('--bg-element', 'Canvas')
  const accent = themeColor('--accent', 'Highlight')
  const text = bodyTextColor()

  return {
    background: 'transparent',
    mainBkg: elementBackground,
    primaryColor: elementBackground,
    primaryTextColor: text,
    primaryBorderColor: accent,
    lineColor: accent,
    edgeLabelBackground: 'transparent',
    textColor: text,
    fontFamily: 'Inter, ui-sans-serif, system-ui, sans-serif',
  }
}

function getMermaidApi() {
  if (!mermaidApiPromise) {
    mermaidApiPromise = import('mermaid').then((module) => {
      const mermaid = module.default
      return mermaid
    })
  }
  return mermaidApiPromise
}

function initializeMermaid(mermaid: MermaidApi) {
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: 'strict',
    theme: 'base',
    themeVariables: mermaidThemeVariables(),
  })
}

function statusLabel(status: string) {
  if (status === 'synced') return 'Current view Mermaid block is synced'
  if (status === 'stale') return 'Update current view Mermaid block'
  if (status === 'other') return ''
  if (status === 'unlinked') return ''
  return 'Insert current view as Mermaid block'
}

function statusIcon(status: string) {
  if (status === 'synced') return <CheckCircleIcon />
  if (status === 'stale') return <WarningIcon />
  return null
}

function tldViewIdLabel(code: string) {
  for (const line of code.split(/\r?\n/)) {
    const marker = tldMarkerPattern.exec(line.trim())
    if (!marker) continue
    const view = tldMarkerViewPattern.exec(marker[1] ?? '')
    return view?.[1] ? Number.parseInt(view[1], 10) : null
  }
  return null
}

function viewLabel(blockViewId: number | null, isCurrentViewBlock: boolean, viewNameById?: Map<number, string>) {
  if (blockViewId === null) return null
  const knownViewName = viewNameById?.get(blockViewId)
  if (knownViewName) return knownViewName
  if (isCurrentViewBlock) return 'current view'
  return `view ${blockViewId}`
}

export function MermaidPreview({ code }: MermaidPreviewProps) {
  const {
    blockStatusByCode,
    canEdit,
    currentViewId,
    viewNameById,
    onNavigateToView,
    onSyncCurrentViewMermaidBlock,
  } = useContext(MermaidMarkdownContext)
  const blockIdRef = useRef(0)
  const [renderState, setRenderState] = useState<MermaidRenderState>(() => (
    code.trim() ? { status: 'loading', svg: '', error: null } : { status: 'empty', svg: '', error: null }
  ))
  const syncStatus = blockStatusByCode.get(code) ?? 'unlinked'
  const blockViewId = tldViewIdLabel(code)
  const isCurrentViewBlock = blockViewId !== null && currentViewId !== null && blockViewId === currentViewId
  const syncStatusLabel = statusLabel(syncStatus)
  const syncStatusIcon = statusIcon(syncStatus)
  const canSyncFromStatus = syncStatus === 'stale' && canEdit && !!onSyncCurrentViewMermaidBlock
  const blockViewLabel = viewLabel(blockViewId, isCurrentViewBlock, viewNameById)
  const showNavigateToView = blockViewId !== null && !isCurrentViewBlock && !!onNavigateToView

  useEffect(() => {
    const trimmedCode = code.trim()
    if (!trimmedCode) {
      setRenderState({ status: 'empty', svg: '', error: null })
      return undefined
    }

    if (!blockIdRef.current) blockIdRef.current = ++mermaidRenderId
    const renderRun = ++mermaidRenderId
    let canceled = false
    setRenderState({ status: 'loading', svg: '', error: null })

    void getMermaidApi()
      .then((mermaid) => {
        initializeMermaid(mermaid)
        return mermaid.render(`tld-mermaid-${blockIdRef.current}-${renderRun}`, code)
      })
      .then(({ svg }) => {
        if (canceled) return
        setRenderState({ status: 'ready', svg, error: null })
      })
      .catch((error) => {
        if (canceled) return
        setRenderState({
          status: 'error',
          svg: '',
          error: error instanceof Error ? error.message : String(error),
        })
      })

    return () => {
      canceled = true
    }
  }, [code])

  return (
    <figure
      className={`tld-mermaid-preview tld-mermaid-preview--${syncStatus} ${isCurrentViewBlock ? 'tld-mermaid-preview--current-view' : ''}`}
      data-testid="tld-mermaid-preview"
      data-tld-view-id={blockViewId ?? undefined}
      data-tld-current-view={isCurrentViewBlock ? 'true' : undefined}
    >
      <figcaption className="tld-mermaid-preview__header">
        <span className="tld-mermaid-preview__title">MERMAID</span>
        <span className="tld-mermaid-preview__metadata">
          {syncStatusLabel && syncStatusIcon && (
            <button
              type="button"
              className={`tld-mermaid-preview__status tld-mermaid-preview__status--${syncStatus}`}
              aria-label={syncStatusLabel}
              title={syncStatusLabel}
              disabled={!canSyncFromStatus}
              onClick={() => {
                if (canSyncFromStatus) void onSyncCurrentViewMermaidBlock?.()
              }}
            >
              {syncStatusIcon}
            </button>
          )}
          {blockViewLabel && (
            <span className="tld-mermaid-preview__meta">
              <span className="tld-mermaid-preview__view-icon" aria-hidden="true">
                <NavigationIcon size={12} strokeWidth={2.1} />
              </span>
              <span className="tld-mermaid-preview__view-name">{blockViewLabel}</span>
            </span>
          )}
        </span>
        {showNavigateToView && (
          <button
            type="button"
            className="tld-mermaid-preview__navigate"
            data-testid="tld-mermaid-navigate-view"
            aria-label="Open in Editor"
            title="Open in Editor"
            onClick={() => {
              if (blockViewId !== null) onNavigateToView?.(blockViewId)
            }}
          >
            <EditIcon boxSize="14px" />
          </button>
        )}
      </figcaption>
      {renderState.status === 'empty' ? (
        <div className="tld-mermaid-preview__empty">Empty Mermaid block</div>
      ) : renderState.status === 'error' ? (
        <pre className="tld-mermaid-preview__error">{renderState.error}</pre>
      ) : (
        <div className="tld-mermaid-preview__body" aria-busy={renderState.status === 'loading'}>
          {renderState.status === 'loading' ? (
            <span className="tld-mermaid-preview__loading">Rendering diagram...</span>
          ) : (
            <div dangerouslySetInnerHTML={{ __html: renderState.svg }} />
          )}
        </div>
      )}
    </figure>
  )
}
