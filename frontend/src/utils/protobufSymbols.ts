import type { SubSymbol } from './subElements'

/** Protobuf declaration ranges, with comments and quoted strings excluded from brace matching. */
export function protobufSymbols(text: string): SubSymbol[] {
  const masked = text.replace(/\/\*[\s\S]*?\*\/|\/\/[^\n]*|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'/g, value => value.replace(/[^\n]/g, ' '))
  const symbols: SubSymbol[] = []
  const stack: Array<{ symbol: SubSymbol; depth: number }> = []
  const tokens = masked.matchAll(/\b(message|enum|service|rpc|oneof)\s+([A-Za-z_][A-Za-z_0-9]*)|[{};]/g)
  let depth = 0, line = 1, offset = 0
  let pending: SubSymbol | undefined
  for (const token of tokens) {
    for (; offset < token.index; offset++) if (masked[offset] === '\n') line++
    if (token[1]) {
      const owner = stack[stack.length - 1]?.symbol.qualifiedName
      pending = { name: token[2], type: token[1], qualifiedName: owner ? `${owner}.${token[2]}` : token[2], startLine: line, endLine: line }
      symbols.push(pending)
    } else if (token[0] === '{') {
      depth++
      if (pending) { stack.push({ symbol: pending, depth }); pending = undefined }
    } else if (token[0] === '}') {
      if (stack[stack.length - 1]?.depth === depth) stack.pop()!.symbol.endLine = line
      depth--
    } else if (pending) { pending.endLine = line; pending = undefined }
  }
  return symbols
}
