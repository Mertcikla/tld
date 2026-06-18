export type SourceAnchor =
  | { kind: 'none' }
  | { kind: 'line'; startLine: number; endLine: number }
  | { kind: 'symbol'; nodeType: string; symbolName: string }

export interface ParsedSourceLink {
  basePath: string
  anchor: SourceAnchor
}

const lineAnchorPattern = /^L([1-9]\d*)(?:-L?([1-9]\d*))?$/
const symbolAnchorPattern = /^([A-Za-z_][A-Za-z0-9_]*):(.+)$/

function safeDecodeURIComponent(value: string) {
  try {
    return decodeURIComponent(value)
  } catch {
    return value
  }
}

export function sourceBasePath(link: string | null | undefined) {
  const value = link ?? ''
  const hashIdx = value.indexOf('#')
  return hashIdx >= 0 ? value.slice(0, hashIdx) : value
}

export function parseSourceAnchor(anchorText: string | null | undefined): SourceAnchor {
  const text = (anchorText ?? '').trim()
  if (!text) return { kind: 'none' }

  const lineMatch = text.match(lineAnchorPattern)
  if (lineMatch) {
    const startLine = Number(lineMatch[1])
    const rawEndLine = lineMatch[2] ? Number(lineMatch[2]) : startLine
    const endLine = Number.isFinite(rawEndLine) && rawEndLine >= startLine ? rawEndLine : startLine
    return { kind: 'line', startLine, endLine }
  }

  const symbolMatch = text.match(symbolAnchorPattern)
  if (symbolMatch) {
    const nodeType = symbolMatch[1].trim()
    const symbolName = safeDecodeURIComponent(symbolMatch[2].trim())
    if (nodeType && symbolName) return { kind: 'symbol', nodeType, symbolName }
  }

  try {
    const parsed = JSON.parse(text)
    if (typeof parsed.startLine === 'number' && parsed.startLine > 0) {
      const endLine = typeof parsed.endLine === 'number' && parsed.endLine >= parsed.startLine
        ? parsed.endLine
        : parsed.startLine
      return { kind: 'line', startLine: parsed.startLine, endLine }
    }
    if (typeof parsed.name === 'string' && parsed.name && typeof parsed.type === 'string' && parsed.type) {
      return { kind: 'symbol', nodeType: parsed.type, symbolName: parsed.name }
    }
  } catch {
    // Legacy JSON anchors are optional.
  }

  return { kind: 'none' }
}

export function parseSourceLink(link: string | null | undefined): ParsedSourceLink {
  const value = link ?? ''
  const hashIdx = value.indexOf('#')
  if (hashIdx < 0) return { basePath: value, anchor: { kind: 'none' } }
  return {
    basePath: value.slice(0, hashIdx),
    anchor: parseSourceAnchor(value.slice(hashIdx + 1)),
  }
}

export function formatLineSourceLink(filePath: string, line: number) {
  const basePath = sourceBasePath(filePath).trim()
  if (!basePath || !Number.isFinite(line) || line <= 0) return basePath
  return `${basePath}#L${Math.trunc(line)}`
}

export function formatSymbolSourceLink(filePath: string, nodeType: string, symbolName: string) {
  const basePath = sourceBasePath(filePath).trim()
  const cleanNodeType = nodeType.trim()
  const cleanSymbolName = symbolName.trim()
  if (!basePath || !cleanNodeType || !cleanSymbolName) return basePath
  return `${basePath}#${cleanNodeType}:${encodeURIComponent(cleanSymbolName)}`
}

export function sourceAnchorLabel(anchor: SourceAnchor) {
  switch (anchor.kind) {
    case 'line':
      return `L${anchor.startLine}`
    case 'symbol':
      return anchor.symbolName
    default:
      return ''
  }
}
