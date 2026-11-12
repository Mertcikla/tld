import { expect, it } from 'vitest'
import { highlightSource } from './subElementSyntax'

it('highlights a selected range with the language parser and preserves its text', async () => {
  const code = 'const n = 1;\nfunction findUser(id: string) {\n  return "hello";\n}'
  const lines = await highlightSource(code, 'users.ts', 2, 3)
  expect(lines.map(tokens => tokens.map(t => t.text).join(''))).toEqual(code.split('\n').slice(1, 3))
  expect(lines[0].some(t => t.text === 'function' && t.color === '#c678dd')).toBe(true)
  expect(lines[1].some(t => t.text.includes('hello') && t.color === '#98c379')).toBe(true)
})

it('highlights protobuf files as source code', async () => {
  const text = 'syntax = "proto3";\nmessage User {\n  string id = 1;\n}'
  const lines = await highlightSource(text, 'users.proto', 1, 4)
  expect(lines.map(tokens => tokens.map(token => token.text).join(''))).toEqual(text.split('\n'))
  expect(lines[0].some(token => token.color === '#98c379')).toBe(true)
  expect(lines[1].some(token => token.color === '#c678dd')).toBe(true)
})
