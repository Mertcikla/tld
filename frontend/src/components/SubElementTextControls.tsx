import { subElementControlSurface, subElementControlButton } from './subElementControlStyles'
import { HStack, IconButton, Text, Tooltip } from '@chakra-ui/react'

export default function SubElementTextControls({ fontSize, lineNumbers, onFontSize, onLineNumbers }: {
  fontSize: number; lineNumbers: boolean; onFontSize: (size: number) => void; onLineNumbers: () => void
}) {
  return <HStack className="nodrag nopan" position="absolute" top="calc(100% + 4px)" left={0} spacing="1px" zIndex={12}
    {...subElementControlSurface} p={1} onPointerDown={event => event.stopPropagation()} onClick={event => event.stopPropagation()}>
    <Tooltip label={`Decrease font size (${fontSize}px)`}><IconButton aria-label="Decrease code font size" {...subElementControlButton} isDisabled={fontSize <= 8} onClick={() => onFontSize(fontSize - 1)} icon={<Text fontSize="10px">A−</Text>} /></Tooltip>
    <Tooltip label={`Increase font size (${fontSize}px)`}><IconButton aria-label="Increase code font size" {...subElementControlButton} isDisabled={fontSize >= 14} onClick={() => onFontSize(fontSize + 1)} icon={<Text fontSize="10px">A+</Text>} /></Tooltip>
    <Tooltip label="Line numbers"><IconButton aria-label="Toggle code line numbers" aria-pressed={lineNumbers} {...subElementControlButton} onClick={onLineNumbers} icon={<Text fontSize="12px">#</Text>} /></Tooltip>
  </HStack>
}
