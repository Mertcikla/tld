import type { CSSProperties } from 'react'

export const TEMPORARY_CONNECTOR_STROKE = 'var(--accent)'
export const TEMPORARY_CONNECTOR_STROKE_WIDTH = 2
export const TEMPORARY_CONNECTOR_STROKE_DASHARRAY = '6 5'

export const TEMPORARY_CONNECTOR_PATH_STYLE: CSSProperties = {
  stroke: TEMPORARY_CONNECTOR_STROKE,
  strokeWidth: TEMPORARY_CONNECTOR_STROKE_WIDTH,
  strokeDasharray: TEMPORARY_CONNECTOR_STROKE_DASHARRAY,
  opacity: 1,
}

export const TEMPORARY_CONNECTOR_EDGE_STYLE: CSSProperties = {
  ...TEMPORARY_CONNECTOR_PATH_STYLE,
  pointerEvents: 'none',
}
