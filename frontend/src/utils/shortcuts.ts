import { isMacPlatform } from '../config/runtime'

// Maps shorthand tokens to the key label for the current platform. `mod`
// resolves to ⌘ on macOS and Ctrl elsewhere, matching how the shortcuts are
// actually bound (metaKey || ctrlKey).
const SHORTCUT_TOKENS: Record<string, { mac: string; other: string }> = {
  mod: { mac: '⌘', other: 'Ctrl' },
  cmd: { mac: '⌘', other: 'Ctrl' },
  ctrl: { mac: '⌃', other: 'Ctrl' },
  shift: { mac: '⇧', other: 'Shift' },
  alt: { mac: '⌥', other: 'Alt' },
  option: { mac: '⌥', other: 'Alt' },
  enter: { mac: '↵', other: '↵' },
}

export function resolveShortcutTokens(keys: string | string[], isMac: boolean = isMacPlatform): string[] {
  return (Array.isArray(keys) ? keys : [keys]).map((token) => {
    const mapping = SHORTCUT_TOKENS[token.toLowerCase()]
    if (!mapping) return token
    return isMac ? mapping.mac : mapping.other
  })
}
