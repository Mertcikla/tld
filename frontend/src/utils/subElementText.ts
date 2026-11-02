import type { SourceToken } from './subElementSource'

export const SUB_CODE_FONT_FAMILY = 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace'
export const SUB_CODE_TEXT_X = 54
export const SUB_SYMBOL_PADDING = 8
export const SUB_SYMBOL_CONNECTOR_GAP = 64
export const subCodeFont = (size = 12) => `${size}px ${SUB_CODE_FONT_FAMILY}`

export function expandCodeTabs(text: string, startColumn = 0) {
  let column = startColumn
  return text.replace(/[^\t]+|\t/g, part => {
    if (part !== '\t') { column += part.length; return part }
    const spaces = 4 - column % 4
    column += spaces
    return ' '.repeat(spaces)
  })
}

export function expandCodeTokens(tokens: SourceToken[]) {
  let column = 0
  return tokens.map(token => {
    const text = expandCodeTabs(token.text, column)
    column += text.length
    return { ...token, text }
  })
}

let context: CanvasRenderingContext2D | null | undefined
export function measureSubText(text: string, size = 12) {
  if (context === undefined && typeof document !== 'undefined' && typeof document.createElement === 'function') context = document.createElement('canvas').getContext('2d')
  if (context) { context.font = subCodeFont(size); return context.measureText(text).width }
  return text.length * size * 0.6
}
