import React from 'react'
import { Box, BoxProps } from '@chakra-ui/react'
import { resolveShortcutTokens } from '../utils/shortcuts'

const kbdBoxProps = {
  as: 'span',
  display: 'inline-flex',
  alignItems: 'center',
  justifyContent: 'center',
  px: 1.5,
  py: 0.5,
  bg: 'whiteAlpha.300',
  rounded: 'sm',
  fontSize: '8px',
  fontWeight: 'bold',
  color: 'whiteAlpha.900',
  flexShrink: 0,
  ml: 2,
} as const

export function KbdHint({ children, ...props }: { children: string } & BoxProps) {
  return (
    <Box {...kbdBoxProps} {...props}>
      {children}
    </Box>
  )
}

// Renders a keyboard shortcut as a single KbdHint key. Tokens are laid out in
// their own flex-centered spans so modifier glyphs (e.g. ⌘) and letters
// optically align regardless of font metrics, while still reading as one key.
// Pass joined tokens such as `['mod', 'K']`, or a single string for a plain key.
export function ShortcutHint({ keys, ...props }: { keys: string | string[] } & BoxProps) {
  const tokens = resolveShortcutTokens(keys)
  return (
    <Box {...kbdBoxProps} ml={0} {...props}>
      <Box as="span" display="inline-flex" alignItems="center" gap="2px">
        {tokens.map((token, index) => (
          <Box as="span" key={`${token}-${index}`} display="inline-flex" alignItems="center">{token}</Box>
        ))}
      </Box>
    </Box>
  )
}
