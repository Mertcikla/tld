import { describe, expect, it } from 'vitest'
import { resolveShortcutTokens } from './shortcuts'

describe('resolveShortcutTokens', () => {
  it('renders modifier glyphs on macOS', () => {
    expect(resolveShortcutTokens(['mod', 'K'], true)).toEqual(['⌘', 'K'])
    expect(resolveShortcutTokens(['shift', 'R'], true)).toEqual(['⇧', 'R'])
    expect(resolveShortcutTokens(['alt', 'Enter'], true)).toEqual(['⌥', '↵'])
  })

  it('renders word modifiers off macOS', () => {
    expect(resolveShortcutTokens(['mod', 'K'], false)).toEqual(['Ctrl', 'K'])
    expect(resolveShortcutTokens(['shift', 'R'], false)).toEqual(['Shift', 'R'])
    expect(resolveShortcutTokens(['alt', 'Enter'], false)).toEqual(['Alt', '↵'])
  })

  it('passes through plain keys and accepts a single string', () => {
    expect(resolveShortcutTokens('C')).toEqual(['C'])
    expect(resolveShortcutTokens(['+/-'], true)).toEqual(['+/-'])
  })
})
