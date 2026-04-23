/**
 * VS Code variant of CodePreviewPanel.
 * Instead of fetching code from GitHub and rendering it inline, we show a
 * single "Open in Editor" button that navigates VS Code to the linked file.
 * CodeMirror + tree-sitter WASM are excluded from the VS Code bundle entirely.
 */
import { Box, Button, HStack, Text } from '@chakra-ui/react'
import { ExternalLinkIcon } from '@chakra-ui/icons'
import { vscodeBridge } from '../lib/vscodeBridge'
import type { PlacedElement } from '../types'

interface Props {
  element: PlacedElement | null
  onClose: () => void
}

export default function CodePreviewPanel({ element, onClose }: Props) {
  if (!element?.file_path) return null

  const filePath = element.file_path
  const hashIdx = filePath.indexOf('#')
  const fp = hashIdx >= 0 ? filePath.slice(0, hashIdx) : filePath
  let symbolName: string | undefined
  let symbolKind: string | undefined
  let startLine: number | undefined
  if (hashIdx >= 0) {
    try {
      const p = JSON.parse(filePath.slice(hashIdx + 1))
      symbolName = typeof p.name === 'string' ? p.name : undefined
      symbolKind = typeof p.type === 'string' ? p.type : undefined
      startLine = typeof p.startLine === 'number' ? p.startLine : undefined
    } catch { /* intentionally empty */ }
  }

  const handleOpen = () => {
    vscodeBridge.postMessage({ type: 'open-file', filePath: fp, startLine, symbolName, symbolKind })
  }

  return (
    <Box
      position="absolute"
      left={4}
      top={4}
      bottom={4}
      w="280px"
      bg="var(--bg-panel)"
      border="1px solid"
      borderColor="whiteAlpha.100"
      rounded="xl"
      display="flex"
      flexDirection="column"
      zIndex={20}
      overflow="hidden"
    >
      <HStack px={3} py={2} borderBottom="1px solid" borderColor="whiteAlpha.100" justify="space-between">
        <Text fontSize="12px" fontWeight="semibold" color="gray.200" isTruncated>
          Source Link
        </Text>
        <Button variant="ghost" size="xs" onClick={onClose} px={1} color="gray.500">
          ✕
        </Button>
      </HStack>

      <Box px={3} py={3} flex={1} display="flex" flexDirection="column" gap={2}>
        {symbolName && (
          <Text fontSize="12px" color="gray.300" fontWeight="medium">{symbolName}</Text>
        )}
        <Text fontSize="11px" color="gray.500" isTruncated title={fp}>{fp}</Text>
        {typeof startLine === 'number' && (
          <Text fontSize="10px" color="gray.600">Line {startLine}</Text>
        )}
        <Button
          mt={2}
          size="sm"
          variant="outline"
          colorScheme="blue"
          leftIcon={<ExternalLinkIcon />}
          onClick={handleOpen}
        >
          Open in Editor
        </Button>
      </Box>
    </Box>
  )
}
