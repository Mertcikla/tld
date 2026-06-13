import { useContext, useEffect, useRef, useState } from 'react'
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

function getMermaidApi() {
  if (!mermaidApiPromise) {
    mermaidApiPromise = import('mermaid').then((module) => {
      const mermaid = module.default
      mermaid.initialize({
        startOnLoad: false,
        securityLevel: 'strict',
        theme: 'base',
        themeVariables: {
          background: '#0d121e',
          mainBkg: '#2d3748',
          primaryColor: '#2d3748',
          primaryTextColor: '#eff6ff',
          primaryBorderColor: '#63b3ed',
          lineColor: '#63b3ed',
          secondaryColor: '#1f2937',
          tertiaryColor: '#171923',
          clusterBkg: '#1f2937',
          clusterBorder: '#63b3ed',
          edgeLabelBackground: '#171923',
          nodeBorder: '#63b3ed',
          textColor: '#eff6ff',
          fontFamily: 'Inter, ui-sans-serif, system-ui, sans-serif',
        },
      })
      return mermaid
    })
  }
  return mermaidApiPromise
}


function statusLabel(status: string) {
  if (status === 'synced') return 'synced'
  if (status === 'stale') return 'stale'
  if (status === 'other') return 'linked elsewhere'
  if (status === 'unlinked') return ' '
  return 'not inserted'
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

function viewLabel(blockViewId: number | null, isCurrentViewBlock: boolean) {
  if (blockViewId === null) return 'unlinked'
  if (isCurrentViewBlock) return 'current view'
  return `view ${blockViewId}`
}

export function MermaidPreview({ code }: MermaidPreviewProps) {
  const { blockStatusByCode, currentViewId, currentViewName } = useContext(MermaidMarkdownContext)
  const blockIdRef = useRef(0)
  const [renderState, setRenderState] = useState<MermaidRenderState>(() => (
    code.trim() ? { status: 'loading', svg: '', error: null } : { status: 'empty', svg: '', error: null }
  ))
  const syncStatus = blockStatusByCode.get(code) ?? 'unlinked'
  const blockViewId = tldViewIdLabel(code)
  const isCurrentViewBlock = blockViewId !== null && currentViewId !== null && blockViewId === currentViewId
  const title = isCurrentViewBlock && currentViewName?.trim() ? currentViewName.trim() : 'Mermaid'
  const blockViewLabel = viewLabel(blockViewId, isCurrentViewBlock)

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
      .then((mermaid) => mermaid.render(`tld-mermaid-${blockIdRef.current}-${renderRun}`, code))
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
    <figure className={`tld-mermaid-preview tld-mermaid-preview--${syncStatus} ${isCurrentViewBlock ? 'tld-mermaid-preview--current-view' : ''}`}>
      <figcaption className="tld-mermaid-preview__header">
        <span className="tld-mermaid-preview__title">{title}</span>
        <span className={`tld-mermaid-preview__status tld-mermaid-preview__status--${syncStatus}`}>
          {statusLabel(syncStatus)}
        </span>
        <span className="tld-mermaid-preview__meta">{blockViewLabel}</span>
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
