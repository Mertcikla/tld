import { expect, it } from 'vitest'
import type { SubSource } from './subElementSource'
import { symbolSignature } from './subElementSignatures'
import { buildSymbolPresentation } from './subElementSymbols'
import { measureSubText } from './subElementText'
import { symbolHandle } from './subElements'

const document = (lines: string[], startLine = 1): SubSource => ({ status: 'ready', lines, tokens: [], symbols: [], startLine, endLine: startLine + lines.length - 1 })

it('shows the full Go type declaration with its existing syntax colors and no body', () => {
  const source = document(['type ClassMember struct {', '\tID string', '}'], 18)
  const symbol = { name: 'ClassMember', type: 'type_spec', startLine: 18, endLine: 20 }
  source.tokens = [[{ text: 'type', color: '#c678dd' }, { text: ' ', color: '#abb2bf' }, { text: 'ClassMember', color: '#e5c07b' }, { text: ' struct {', color: '#c678dd' }]]
  const signature = symbolSignature(source, symbol)
  expect(signature.text).toBe('type ClassMember struct')
  expect(signature.tokens).toContainEqual({ text: 'ClassMember', color: '#e5c07b' })
  expect(signature.tokens.map(token => token.text).join('')).toBe(signature.text)
})

it('keeps multiline receiver, parameter and return declarations, including braces in strings and parameter types', () => {
  const source = document(['func (s *Store) Find(', '\tid string,', '\tfilter func(struct{ ID string }) bool,', ') (*User, error) {', '\treturn nil, nil', '}'])
  expect(symbolSignature(source, { name: 'Find', type: 'method_declaration', startLine: 1, endLine: 6 }).text).toBe('func (s *Store) Find( id string, filter func(struct{ ID string }) bool, ) (*User, error)')
  const js = document(['function find(id = "{", options = { cache: true }) { return id }'])
  expect(symbolSignature(js, { name: 'find', type: 'function_declaration', startLine: 1, endLine: 1 }).text).toBe('function find(id = "{", options = { cache: true })')
})

it('refreshes cached signatures when asynchronous highlighting arrives or the source changes', () => {
  const source = document(['type User struct { id string }'])
  const symbol = { name: 'User', type: 'type_spec', startLine: 1, endLine: 1 }
  const plain = symbolSignature(source, symbol)
  expect(symbolSignature(source, symbol)).toBe(plain)
  expect(symbolSignature(source, { ...symbol })).toBe(plain)
  source.tokens = [[{ text: source.lines[0], color: 'gold' }]]
  expect(symbolSignature(source, symbol).tokens[0].color).toBe('gold')
  source.lines = ['type User interface { Save() }']
  source.tokens = []
  expect(symbolSignature(source, symbol).text).toBe('type User interface')
})

it('uses a source line for unparsed ranges and falls back to the symbol name when text is unavailable', () => {
  const source = document(['', 'CREATE TABLE users (', 'id TEXT', ');'], 2)
  const symbol = { name: 'users.sql', startLine: 2, endLine: 5 }
  expect(symbolSignature(source, symbol).text).toBe('CREATE TABLE users (')
  expect(symbolSignature(document([]), symbol).text).toBe('users.sql')
})

it('encloses long collapsed signatures and keeps their original semantic connection IDs', () => {
  const source = document([`func Find(${Array.from({ length: 20 }, (_, i) => `argument${i} string`).join(', ')}) error {`, '}'])
  const symbol = { name: 'Find', type: 'function_declaration', startLine: 1, endLine: 2 }
  source.symbols = [symbol]
  const id = symbolHandle('right', symbol)
  const model = buildSymbolPresentation({ kind: 'code', source, detail: 'outline', width: 400, height: 240, connected: [id] })
  const area = model.areas[0]
  expect(area.signature?.text).toBe(source.lines[0].replace(' {', ''))
  expect(area.width).toBeGreaterThan(measureSubText(area.signature!.text))
  expect(model.contentWidth).toBeGreaterThan(area.x + area.width)
  expect(model.endpoints.get(id)?.area).toBe(area)
})
