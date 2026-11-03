import type { SourceToken, SubSource } from './subElementSource'
import type { SubSymbol } from './subElements'
import { expandCodeTokens } from './subElementText'

export interface SymbolSignature { text: string; tokens: SourceToken[] }
const cache = new WeakMap<SubSource, { lines: string[]; tokens: SourceToken[][]; signatures: Map<string, SymbolSignature> }>()

// Keep multiline parameter lists, receivers and return types, stopping before the body.
function headerEnd(text: string) {
  let parentheses = 0, brackets = 0, braces = 0, quote = '', escaped = false
  for (let index = 0; index < text.length; index++) {
    const char = text[index]
    if (quote) {
      if (escaped) escaped = false
      else if (char === '\\') escaped = true
      else if (char === quote) quote = ''
      continue
    }
    if (char === '"' || char === "'" || char === '`') { quote = char; continue }
    if (char === '(') parentheses++
    else if (char === ')') parentheses--
    else if (char === '[') brackets++
    else if (char === ']') brackets--
    else if (char === '{') {
      if (!parentheses && !brackets && !braces) return index
      braces++
    } else if (char === '}') braces--
    else if (!parentheses && !brackets && !braces && (char === ';' || char === '\n')) return index
  }
  return text.length
}

export function symbolSignature(source: SubSource, symbol: SubSymbol): SymbolSignature {
  let document = cache.get(source)
  if (!document || document.lines !== source.lines || document.tokens !== source.tokens) {
    document = { lines: source.lines, tokens: source.tokens, signatures: new Map() }
    cache.set(source, document)
  }
  const key = JSON.stringify([symbol.type, symbol.name, symbol.startLine, symbol.endLine])
  const existing = document.signatures.get(key)
  if (existing) return existing
  let first = Math.max(0, symbol.startLine - source.startLine)
  while (first < symbol.endLine - source.startLine && !source.lines[first]?.trim()) first++
  // A file/range without a parsed symbol uses its first nonempty source line.
  const lines = source.lines.slice(first, symbol.type ? symbol.endLine - source.startLine + 1 : first + 1)
  const end = headerEnd(lines.join('\n'))
  const tokens: SourceToken[] = []
  let offset = 0
  for (let index = 0; index < lines.length && offset <= end; index++) {
    const line = lines[index].slice(0, Math.max(0, end - offset))
    const from = line.length - line.trimStart().length, to = line.trimEnd().length
    if (to > from) {
      if (tokens.length) tokens.push({ text: ' ', color: '#abb2bf' })
      let column = 0
      for (const token of source.tokens[first + index]?.length ? source.tokens[first + index] : [{ text: lines[index], color: '#abb2bf' }]) {
        const text = token.text.slice(Math.max(0, from - column), Math.max(0, to - column))
        if (text) tokens.push({ text, color: token.color })
        column += token.text.length
        if (column >= to) break
      }
    }
    offset += lines[index].length + 1
  }
  const expanded = tokens.length ? expandCodeTokens(tokens) : [{ text: symbol.name, color: '#61afef' }]
  const signature = { text: expanded.map(token => token.text).join('') || symbol.name, tokens: expanded }
  if (!expanded.length) expanded.push({ text: signature.text, color: '#abb2bf' })
  document.signatures.set(key, signature)
  return signature
}
