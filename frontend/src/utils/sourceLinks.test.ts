import { describe, expect, it } from 'vitest'
import {
  formatLineSourceLink,
  formatSymbolSourceLink,
  parseSourceLink,
  sourceAnchorLabel,
} from './sourceLinks'

describe('source links', () => {
  it('parses line anchors', () => {
    const parsed = parseSourceLink('grpc/server.go#L18')

    expect(parsed.basePath).toBe('grpc/server.go')
    expect(parsed.anchor).toEqual({ kind: 'line', startLine: 18, endLine: 18 })
    expect(sourceAnchorLabel(parsed.anchor)).toBe('L18')
  })

  it('formats and parses native tree-sitter symbol anchors', () => {
    const link = formatSymbolSourceLink('grpc/server.go', 'method_declaration', 'Server.Listen')

    expect(link).toBe('grpc/server.go#method_declaration:Server.Listen')
    expect(parseSourceLink(link)).toEqual({
      basePath: 'grpc/server.go',
      anchor: { kind: 'symbol', nodeType: 'method_declaration', symbolName: 'Server.Listen' },
    })
  })

  it('encodes symbol names without changing node type text', () => {
    const link = formatSymbolSourceLink('src/api.ts#L4', 'function_declaration', 'listen:public route')

    expect(link).toBe('src/api.ts#function_declaration:listen%3Apublic%20route')
    expect(parseSourceLink(link).anchor).toEqual({
      kind: 'symbol',
      nodeType: 'function_declaration',
      symbolName: 'listen:public route',
    })
  })

  it('formats line anchors from plain or already-anchored paths', () => {
    expect(formatLineSourceLink('grpc/server.go', 18)).toBe('grpc/server.go#L18')
    expect(formatLineSourceLink('grpc/server.go#function_declaration:Listen', 22)).toBe('grpc/server.go#L22')
  })

  it('reads legacy JSON anchors', () => {
    expect(parseSourceLink('src/app.ts#{"name":"App","type":"function_declaration"}')).toEqual({
      basePath: 'src/app.ts',
      anchor: { kind: 'symbol', nodeType: 'function_declaration', symbolName: 'App' },
    })
    expect(parseSourceLink('src/app.ts#{"startLine":7,"endLine":9}')).toEqual({
      basePath: 'src/app.ts',
      anchor: { kind: 'line', startLine: 7, endLine: 9 },
    })
  })
})
