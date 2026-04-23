import { Box, HStack, Text } from '@chakra-ui/react'
import { keyframes } from '@emotion/react'
import { ZoomInIcon } from './Icons'

interface Props {
  isVisible: boolean
}

const pulseGlow = keyframes`
  0% { box-shadow: 0 0 0 0 rgba(var(--accent-rgb), 0.4); }
  70% { box-shadow: 0 0 0 20px rgba(var(--accent-rgb), 0); }
  100% { box-shadow: 0 0 0 0 rgba(var(--accent-rgb), 0); }
`

export default function MiniZoomOnboarding({ isVisible }: Props) {
  return (
    <Box
      position="absolute"
      top={{ base: '30px', md: '50px' }}
      left="50%"
      transform={isVisible ? 'translateX(-50%) translateY(0) scale(1)' : 'translateX(-50%) translateY(-20px) scale(0.95)'}
      zIndex={100}
      opacity={isVisible ? 1 : 0}
      transition="all 0.8s cubic-bezier(0.16, 1, 0.3, 1)"
      pointerEvents="none"
    >
      <Box
        className="glass"
        px={6}
        py={4}
        borderRadius="12px"
        animation={isVisible ? `${pulseGlow} 3s infinite` : 'none'}
        position="relative"
        overflow="hidden"
        border="1.5px solid rgba(var(--accent-rgb), 0.3)"
      >
        {/* Subtle accent bar for visual continuity */}
        <Box
          position="absolute"
          top={0}
          left={0}
          w="4px"
          h="100%"
          bg="var(--accent)"
          opacity={0.8}
        />
        
        <HStack spacing={5} pl={3}>
          <Box color="var(--accent)">
            <ZoomInIcon size={24} />
          </Box>
          <Box>
            <Text 
              fontSize="10px" 
              color="var(--accent)" 
              fontWeight="900" 
              letterSpacing="0.15em" 
              textTransform="uppercase" 
              mb={0.5}
              opacity={0.9}
            >
              Pro Tip
            </Text>
            <Text 
              fontSize="15px" 
              color="white" 
              fontWeight="600" 
              whiteSpace="nowrap"
              letterSpacing="-0.01em"
            >
              Scroll or pinch to dive into nodes
            </Text>
          </Box>
        </HStack>
      </Box>
    </Box>
  )
}
