import { memo } from 'react'
import { Handle, Position } from 'reactflow'
import { Box } from '@chakra-ui/react'

interface Props {
  data: {
    width: number
    height: number
    parentName: string
    onNavigateToDiagram: () => void
  }
}

function ContextBoundaryElement({ data }: Props) {
  return (
    <Box
      w={`${data.width}px`}
      h={`${data.height}px`}
      border="2px dashed rgba(255, 255, 255, 0.15)"
      rounded="3xl"
      position="relative"
      pointerEvents="none"
      bg="rgba(0, 0, 0, 0.05)"
    >
      <Handle type="source" position={Position.Top} id="top" style={{ opacity: 0, pointerEvents: 'none' }} isConnectable={false} />
      <Handle type="source" position={Position.Bottom} id="bottom" style={{ opacity: 0, pointerEvents: 'none' }} isConnectable={false} />
      <Handle type="source" position={Position.Left} id="left" style={{ opacity: 0, pointerEvents: 'none' }} isConnectable={false} />
      <Handle type="source" position={Position.Right} id="right" style={{ opacity: 0, pointerEvents: 'none' }} isConnectable={false} />


    </Box>
  )
}

export default memo(ContextBoundaryElement)
