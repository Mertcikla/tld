import type Parser from 'web-tree-sitter'
import { describe, expect, it } from 'vitest'
import { extractSymbols } from './treesitter'
import { buildSymbolPresentation } from './subElementSymbols'
import type { SubSource } from './subElementSource'

// The syntax-node surface used by extractSymbols, without loading WASM.
function node(type: string, start: number, end: number, children: Parser.SyntaxNode[] = [], name = ''): Parser.SyntaxNode {
  const result = {
    text: '', type, startPosition: { row: start - 1 }, endPosition: { row: end - 1 },
    childCount: children.length, child: (index: number) => children[index] ?? null,
    childForFieldName: (field: string) => field === 'name' && name ? { text: name } : null,
  } as unknown as Parser.SyntaxNode
  for (const child of children) Object.assign(child, { parent: result })
  return result
}

const tree = (rootNode: Parser.SyntaxNode, text = '') => ({ rootNode: Object.assign(rootNode, { text }) }) as Parser.Tree

describe('Python decorated symbols', () => {
  it('records stacked and multiline decorators without changing the signature line', () => {
    const fn = node('function_definition', 29, 33, [], 'create_dashboard')
    const decorated = node('decorated_definition', 22, 33, [node('decorator', 22, 22), node('decorator', 23, 28), fn])
    const plain = node('function_definition', 36, 38, [], 'plain')
    expect(extractSymbols(tree(node('module', 1, 38, [decorated, plain])), 'python')).toEqual([
      { name: 'create_dashboard', qualifiedName: 'create_dashboard', type: 'function_definition', attachmentStartLine: 22, startLine: 29, endLine: 33 },
      { name: 'plain', qualifiedName: 'plain', type: 'function_definition', startLine: 36, endLine: 38 },
    ])
  })

  it('preserves decorated class and method ownership', () => {
    const method = node('decorated_definition', 5, 8, [node('function_definition', 7, 8, [], 'save')])
    const klass = node('decorated_definition', 1, 8, [node('class_definition', 3, 8, [method], 'Dashboard')])
    expect(extractSymbols(tree(node('module', 1, 8, [klass])), 'python')).toEqual([
      { name: 'Dashboard', qualifiedName: 'Dashboard', type: 'class_definition', attachmentStartLine: 1, startLine: 3, endLine: 8 },
      { name: 'save', qualifiedName: 'Dashboard.save', type: 'function_definition', attachmentStartLine: 5, startLine: 7, endLine: 8 },
    ])
  })
})


describe('indexed declaration attachment boundaries', () => {
  it('attaches the leading route comment at line 26 to preview_query_rule_sql', () => {
    const lines = [...Array<string>(25).fill(''),
      '# Registered ahead of GET/PUT/DELETE "/{query_rule_id}" -- no method',
      "# collision today (those aren't POST-registered), but static routes",
      '# before param routes is the safer default regardless.',
      '@router.post("/preview", response_model=TelemetrySqlQueryResult)',
      'async def preview_query_rule_sql(',
      '    payload: QueryRulePreviewRequest,',
      '    service: QueryRuleService = Depends(get_query_rule_service),',
      ') -> TelemetrySqlQueryResult:',
      '    return await service.preview(payload.sql)',
    ]
    const fn = node('function_definition', 30, 34, [], 'preview_query_rule_sql')
    const decorated = node('decorated_definition', 29, 34, [fn])
    const symbols = extractSymbols(tree(node('module', 1, 34, [decorated]), lines.join('\n')), 'python')
    expect(symbols).toEqual([
      { name: 'preview_query_rule_sql', qualifiedName: 'preview_query_rule_sql', type: 'function_definition', attachmentStartLine: 26, startLine: 30, endLine: 34 },
    ])
    const source: SubSource = { status: 'ready', startLine: 1, endLine: lines.length, lines, symbols, tokens: [] }
    for (const detail of ['structure', 'summary', 'outline', 'code'] as const) {
      for (const connectedOnly of [false, true]) {
        // Existing map connectors contain the comment start, decorator start,
        // and function start; all must resolve to the same semantic port.
        const id = 'right-line-26,29,30'
        const model = buildSymbolPresentation({ kind: 'code', source, detail, connectedOnly, connected: [id], width: 600, height: 1000 })
        expect(model.areas.map(area => area.symbol.name)).toEqual(['preview_query_rule_sql'])
        expect(model.rows.some(row => row.symbol?.name.startsWith('Line '))).toBe(false)
        const area = model.areas[0]
        for (const handle of [id, 'right-line-26', 'right-line-29', 'right-line-30']) {
          expect(model.endpoints.get(handle)?.area).toBe(area)
          expect(model.endpoints.get(handle)?.y).toBe(area.y)
        }
        if (detail === 'outline') expect(area.signature?.text).toMatch(/^async def preview_query_rule_sql/)
      }
    }

  })

  it.each([
    ['python', 'function_definition', '# explanation\n# continued\ndef run():\n    pass', 3, 1],
    ['python', 'function_definition', '# separate comment\n\ndef run():\n    pass', 3, undefined],
    ['go', 'function_declaration', '// Run executes the request.\nfunc Run() {}', 2, 1],
    ['typescript', 'function_declaration', '/** Run the request.\n * More detail.\n */\nfunction run() {}', 4, 1],
  ] as const)('matches indexed comment boundaries in %s: %s', (language, type, text, startLine, attachmentStartLine) => {
    const fn = node(type, startLine, text.split('\n').length, [], 'run')
    const symbols = extractSymbols(tree(node('module', 1, fn.endPosition.row + 1, [fn]), text), language)
    expect(symbols).toHaveLength(1)
    expect(symbols[0].startLine).toBe(startLine)
    expect(symbols[0].attachmentStartLine).toBe(attachmentStartLine)
  })
})
