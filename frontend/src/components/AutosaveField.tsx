import { useEffect, useState, type ReactNode } from 'react'
import { Box, HStack, Text } from '@chakra-ui/react'
import { keyframes } from '@emotion/react'
import { editorFeedbackStyles } from '../styles/editorInteractions'

interface Props {
  children: ReactNode
  enabled: boolean
  receipt?: number
  saving?: boolean
  multiline?: boolean
  hasTrailingAction?: boolean
  field: string
}

/** Reserve space so save feedback never covers text or shifts the field. */
const spin = keyframes({ to: { transform: 'rotate(360deg)' } })

export default function AutosaveField({ children, enabled, receipt, saving = false, multiline, hasTrailingAction, field }: Props) {
  const [visibleReceipt, setVisibleReceipt] = useState<number>()
  useEffect(() => {
    setVisibleReceipt(receipt)
    if (receipt === undefined) return
    const timer = setTimeout(() => setVisibleReceipt(undefined), 2000)
    return () => clearTimeout(timer)
  }, [receipt])
  const visible = enabled && (saving || (receipt !== undefined && receipt === visibleReceipt))

  return (
    <Box position="relative" w="full" minW={0} sx={enabled ? {
      '& input, & textarea': { paddingInlineEnd: hasTrailingAction ? '7rem' : '5rem' },
    } : undefined}>
      {children}
      {enabled && (
        <HStack
          data-testid={`autosave-field-${field}`}
          role="status"
          aria-live="polite"
          aria-atomic="true"
          position="absolute"
          right={hasTrailingAction ? '2.5rem' : 2.5}
          top={multiline ? 2 : 0}
          h={multiline ? '18px' : 'full'}
          spacing={1.5}
          color={saving ? 'gray.400' : 'green.400'}
          fontSize="11px"
          pointerEvents="none"
          userSelect="none"
          opacity={visible ? 1 : 0}
          transition="opacity 160ms ease"
          sx={{ '@media (prefers-reduced-motion: reduce)': { transition: 'none' } }}
        >
          {visible && <>
            {saving ? <Box
              as="span"
              data-testid="autosave-field-spinner"
              boxSize="12px"
              flexShrink={0}
              border="1.5px solid currentColor"
              borderTopColor="transparent"
              borderRadius="full"
              aria-hidden="true"
              sx={{ animation: `${spin} 700ms linear infinite`, '@media (prefers-reduced-motion: reduce)': { animation: 'none' } }}
            /> : <Box as="span" display="inline-flex" sx={editorFeedbackStyles} aria-hidden="true">
              <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"><path d="m3 8 3 3 7-7" /></svg>
            </Box>}
            <Text as="span">{saving ? 'Saving…' : 'Saved'}</Text>
          </>}
        </HStack>
      )}
    </Box>
  )
}
