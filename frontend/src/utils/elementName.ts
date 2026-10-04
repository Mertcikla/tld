export const ELEMENT_NAME_FONT_FAMILY = "'Metrophobic', system-ui, -apple-system, sans-serif"
export const ELEMENT_NAME_INSET_RATIO = 0.08
export const ELEMENT_NAME_LINE_HEIGHT = 1.15
export const ELEMENT_NAME_FONT_STEPS = [20, 18, 16, 14, 13]

export interface NameFitResult {
  lines: string[]
  fontSize: number
  truncated: boolean
}

export interface NameFitOptions {
  name: string
  maxWidth: number
  maxLines: number
  fontSizes: number[]
  lineHeight?: number
  bandHeight?: number
  measure: (text: string, fontSize: number) => number
  measureKey?: string
}

const ELISION = '\u2026'
const FIT_CACHE_LIMIT = 2048
const fitCache = new Map<string, NameFitResult>()

const SEPARATOR = /[\s_\-./\\:]/
const UPPER = /[A-Z]/
const LOWER_OR_DIGIT = /[a-z0-9]/
const LOWER = /[a-z]/

type FontsChangedListener = () => void

const fontsChangedListeners = new Set<FontsChangedListener>()

export function resetElementNameFitCache(): void {
  fitCache.clear()
}

function handleElementNameFontsChanged(): void {
  resetElementNameFitCache()
  for (const listener of fontsChangedListeners) listener()
}

if (typeof document !== 'undefined' && typeof document.fonts !== 'undefined') {
  void document.fonts.ready.then(handleElementNameFontsChanged)
  document.fonts.addEventListener?.('loadingdone', handleElementNameFontsChanged)
}

export function subscribeElementNameFontsChanged(listener: FontsChangedListener): () => void {
  fontsChangedListeners.add(listener)
  return () => {
    fontsChangedListeners.delete(listener)
  }
}

export function splitNameSegments(name: string): string[] {
  if (name.length <= 1) return name ? [name] : []
  const segments: string[] = []
  let start = 0
  for (let i = 0; i < name.length - 1; i += 1) {
    const current = name[i]
    const next = name[i + 1]
    let shouldBreak = SEPARATOR.test(current)
    if (!shouldBreak && LOWER_OR_DIGIT.test(current) && UPPER.test(next)) shouldBreak = true
    if (!shouldBreak && UPPER.test(current) && UPPER.test(next)) {
      const after = name[i + 2]
      if (after !== undefined && LOWER.test(after)) shouldBreak = true
    }
    if (shouldBreak) {
      segments.push(name.slice(start, i + 1))
      start = i + 1
    }
  }
  segments.push(name.slice(start))
  return segments.filter((segment) => segment.length > 0)
}

function joinSegments(segments: string[], start: number, end: number): string {
  return segments.slice(start, end).join('').trim()
}

function wrapName(
  name: string,
  maxWidth: number,
  fontSize: number,
  maxLines: number,
  measure: NameFitOptions['measure'],
): string[] | null {
  if (measure(name, fontSize) <= maxWidth) return [name]
  if (maxLines <= 1) return null

  const segments = splitNameSegments(name)
  if (segments.length <= 1) return null

  if (maxLines === 2) {
    let best: { lines: string[]; worst: number; spread: number } | null = null
    for (let i = 1; i < segments.length; i += 1) {
      const left = joinSegments(segments, 0, i)
      const right = joinSegments(segments, i, segments.length)
      if (!left || !right) continue
      const leftWidth = measure(left, fontSize)
      const rightWidth = measure(right, fontSize)
      if (leftWidth > maxWidth || rightWidth > maxWidth) continue
      const worst = Math.max(leftWidth, rightWidth)
      const spread = Math.abs(leftWidth - rightWidth)
      if (!best || worst < best.worst || (worst === best.worst && spread < best.spread)) {
        best = { lines: [left, right], worst, spread }
      }
    }
    return best?.lines ?? null
  }

  const lines: string[] = []
  let current = ''
  for (const segment of segments) {
    const candidate = current + segment
    if (current && measure(candidate.trim(), fontSize) > maxWidth) {
      lines.push(current.trim())
      if (lines.length >= maxLines) return null
      current = segment
    } else {
      current = candidate
    }
  }
  const last = current.trim()
  if (last) lines.push(last)
  return lines.length > 0 && lines.length <= maxLines ? lines : null
}

function elideName(
  name: string,
  maxWidth: number,
  fontSize: number,
  measure: NameFitOptions['measure'],
): string {
  if (!name) return name
  if (measure(ELISION, fontSize) > maxWidth) return ELISION

  const segments = splitNameSegments(name)
  if (segments.length > 2) {
    const first = segments[0].trim()
    const last = segments[segments.length - 1].trim()
    if (first && last) {
      const combined = `${first}${ELISION}${last}`
      if (measure(combined, fontSize) <= maxWidth) return combined
    }
    if (last) {
      const tailOnly = `${ELISION}${last}`
      if (measure(tailOnly, fontSize) <= maxWidth) return tailOnly
    }
  }

  let low = 1
  let high = name.length - 1
  let tailLength = 0
  while (low <= high) {
    const mid = (low + high) >> 1
    if (measure(ELISION + name.slice(name.length - mid), fontSize) <= maxWidth) {
      tailLength = mid
      low = mid + 1
    } else {
      high = mid - 1
    }
  }
  if (tailLength === 0) return ELISION

  const tail = Math.min(tailLength, Math.ceil(name.length / 2))
  low = 1
  high = name.length - tail - 1
  let headLength = 0
  while (low <= high) {
    const mid = (low + high) >> 1
    if (measure(name.slice(0, mid) + ELISION + name.slice(name.length - tail), fontSize) <= maxWidth) {
      headLength = mid
      low = mid + 1
    } else {
      high = mid - 1
    }
  }
  return name.slice(0, headLength) + ELISION + name.slice(name.length - tail)
}

export function fitElementName(options: NameFitOptions): NameFitResult {
  const {
    name,
    maxWidth,
    maxLines,
    fontSizes,
    lineHeight = ELEMENT_NAME_LINE_HEIGHT,
    bandHeight,
    measure,
    measureKey = '',
  } = options

  if (fontSizes.length === 0 || maxWidth <= 0) {
    return { lines: [name], fontSize: fontSizes[fontSizes.length - 1] ?? 0, truncated: false }
  }

  const key = [
    measureKey,
    name,
    maxWidth,
    maxLines,
    fontSizes.join(','),
    lineHeight,
    bandHeight ?? 'none',
  ].join('|')
  const cached = fitCache.get(key)
  if (cached) return cached

  let result: NameFitResult | null = null
  for (const fontSize of fontSizes) {
    const lines = wrapName(name, maxWidth, fontSize, maxLines, measure)
    if (!lines) continue
    if (bandHeight !== undefined && lines.length * fontSize * lineHeight > bandHeight) continue
    result = { lines, fontSize, truncated: false }
    break
  }

  if (!result) {
    const fontSize = fontSizes[fontSizes.length - 1]
    result = { lines: [elideName(name, maxWidth, fontSize, measure)], fontSize, truncated: true }
  }

  fitCache.set(key, result)
  if (fitCache.size > FIT_CACHE_LIMIT) {
    const oldest = fitCache.keys().next().value
    if (oldest !== undefined) fitCache.delete(oldest)
  }
  return result
}

let measureCanvasContext: CanvasRenderingContext2D | null = null

export function measureTextWithCanvas(text: string, fontSize: number): number {
  if (typeof document === 'undefined') return text.length * fontSize * 0.62
  if (!measureCanvasContext) {
    try {
      measureCanvasContext = document.createElement('canvas').getContext('2d')
    } catch {
      measureCanvasContext = null
    }
  }
  if (!measureCanvasContext) return text.length * fontSize * 0.62
  measureCanvasContext.font = `600 ${fontSize}px ${ELEMENT_NAME_FONT_FAMILY}`
  return measureCanvasContext.measureText(text).width
}
