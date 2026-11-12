import { Box, Portal, Text } from '@chakra-ui/react'
import type { usePreviewInteraction } from '../hooks/usePreviewInteraction'

export default function PreviewInteractionHint({ interaction }: { interaction: ReturnType<typeof usePreviewInteraction> }) {
  if (interaction.phase !== 'waiting') return null
  return <Portal><Box ref={interaction.hintRef} position="fixed" left={0} top={0} transform="translate(-50%, calc(-100% - 16px))" w="130px" p="6px" bg="#1b222d" border="1px solid" borderColor="whiteAlpha.200" borderRadius="6px" boxShadow="lg" zIndex={10000} pointerEvents="none">
    <Text fontSize="10px" lineHeight="10px" color="gray.300" textAlign="center" mb="3px">Keep hovering to interact</Text>
    <Box role="progressbar" aria-label="Activating preview interaction" aria-valuemin={0} aria-valuemax={2} h="5px" bg="whiteAlpha.200" overflow="hidden" borderRadius="full">
      <Box h="100%" bg="var(--accent)" transformOrigin="left" animation="preview-hover-progress 1s linear forwards" sx={{ '@keyframes preview-hover-progress': { from: { transform: 'scaleX(0)' }, to: { transform: 'scaleX(1)' } } }} />
    </Box>
  </Box></Portal>
}
