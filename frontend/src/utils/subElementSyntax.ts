import { EditorState } from '@codemirror/state'
import { syntaxTree, ensureSyntaxTree } from '@codemirror/language'
import { languages } from '@codemirror/language-data'
import { highlightTree, classHighlighter } from '@lezer/highlight'
import type { SourceToken } from './subElementSource'

const colors: Record<string, string> = {
  'tok-keyword': '#c678dd', 'tok-string': '#98c379', 'tok-string2': '#98c379',
  'tok-comment': '#7f848e', 'tok-number': '#d19a66', 'tok-bool': '#d19a66',
  'tok-typeName': '#e5c07b', 'tok-className': '#e5c07b', 'tok-propertyName': '#e06c75',
  'tok-variableName': '#abb2bf', 'tok-definition': '#61afef', 'tok-operator': '#56b6c2',
}
export async function highlightSource(text: string, path: string, start: number, end: number): Promise<SourceToken[][]> {
  const language = languages.find(l => l.extensions.includes(path.split('.').pop() ?? ''))
  const support = await language?.load()
  const state = EditorState.create({ doc: text, extensions: support ? [support] : [] })
  const tree = ensureSyntaxTree(state, state.doc.length, 200) ?? syntaxTree(state)
  const spans: Array<{ from: number; to: number; color: string }> = []
  highlightTree(tree, classHighlighter, (from, to, classes) => {
    const names = classes.split(' ')
    const color = names.includes('tok-definition') ? '#61afef' : names.map(c => colors[c]).find(Boolean) ?? '#abb2bf'
    spans.push({ from, to, color })
  })
  let spanIndex = 0
  return Array.from({ length: end - start + 1 }, (_, index) => {
    const line = state.doc.line(start + index)
    const tokens: SourceToken[] = []
    let cursor = line.from
    while (spanIndex < spans.length && spans[spanIndex].to <= line.from) spanIndex++
    for (let i = spanIndex; i < spans.length && spans[i].from < line.to; i++) {
      const span = spans[i]
      const from = Math.max(cursor, span.from), to = Math.min(line.to, span.to)
      if (from > cursor) tokens.push({ text: text.slice(cursor, from), color: '#abb2bf' })
      if (to > from) tokens.push({ text: text.slice(from, to), color: span.color })
      cursor = Math.max(cursor, to)
    }
    if (cursor < line.to) tokens.push({ text: text.slice(cursor, line.to), color: '#abb2bf' })
    return tokens
  })
}
