import { HStack, IconButton, Text, Tooltip } from '@chakra-ui/react'

export default function SubElementTextControls({ fontSize, lineNumbers, onFontSize, onLineNumbers }: {
  fontSize: number; lineNumbers: boolean; onFontSize: (size: number) => void; onLineNumbers: () => void
}) {
  return <HStack className="nodrag nopan" position="absolute" top="calc(100% + 4px)" left={1} spacing="1px" zIndex={12}
    bg="#20252d" borderRadius="5px" p={1} onPointerDown={event => event.stopPropagation()} onClick={event => event.stopPropagation()}>
    <Tooltip label="Decrease font size"><IconButton aria-label="Decrease code font size" variant="ghost" minW="20px" w="20px" h="20px" borderRadius="3px" _hover={{ color: 'gray.100', bg: 'whiteAlpha.200' }} color="gray.500" isDisabled={fontSize <= 8} onClick={() => onFontSize(fontSize - 1)} icon={<Text fontSize="10px">A−</Text>} /></Tooltip>
    <Text fontSize="10px" color="gray.300" minW="29px" textAlign="center" aria-label="Code font size">{fontSize}px</Text>
    <Tooltip label="Increase font size"><IconButton aria-label="Increase code font size" variant="ghost" minW="20px" w="20px" h="20px" borderRadius="3px" _hover={{ color: 'gray.100', bg: 'whiteAlpha.200' }} color="gray.500" isDisabled={fontSize >= 14} onClick={() => onFontSize(fontSize + 1)} icon={<Text fontSize="10px">A+</Text>} /></Tooltip>
    <Tooltip label="Line numbers"><IconButton aria-label="Toggle code line numbers" aria-pressed={lineNumbers} variant="ghost" minW="20px" w="20px" h="20px" borderRadius="3px" _hover={{ color: 'gray.100', bg: 'whiteAlpha.200' }} color={lineNumbers ? 'var(--accent)' : 'gray.500'} bg={lineNumbers ? 'whiteAlpha.100' : 'transparent'} onClick={onLineNumbers} icon={<Text fontSize="12px">#</Text>} /></Tooltip>
  </HStack>
}
