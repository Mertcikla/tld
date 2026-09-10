import type { ReactNode } from 'react'
import { Box, Button, CloseButton, HStack, Text, VStack } from '@chakra-ui/react'

interface Props {
  step: number
  totalSteps: number
  title: string
  body: string
  visual?: ReactNode
  onBack: () => void
  onNext: () => void
  onSelectStep: (index: number) => void
  onDismiss: () => void
}

export default function OnboardingHint({
  step,
  totalSteps,
  title,
  body,
  visual,
  onBack,
  onNext,
  onSelectStep,
  onDismiss,
}: Props) {
  const isLast = step === totalSteps - 1

  return (
    <Box
      position="fixed"
      left={{ base: 3, md: 4 }}
      bottom={{ base: 3, md: 4 }}
      zIndex={2000}
      w="calc(100% - 24px)"
      maxW="320px"
      pointerEvents="none"
    >
      <Box
        className="glass"
        position="relative"
        borderRadius="14px"
        p={4}
        pr={10}
        boxShadow="0 12px 32px rgba(0,0,0,0.45), 0 2px 8px rgba(0,0,0,0.3)"
        pointerEvents="auto"
      >
        <CloseButton
          aria-label="Dismiss hint"
          position="absolute"
          top={2}
          right={2}
          size="sm"
          color="whiteAlpha.700"
          _hover={{ color: 'white', bg: 'whiteAlpha.200' }}
          onClick={onDismiss}
        />

        <Text
          fontSize="10px"
          color="var(--accent)"
          fontWeight="900"
          letterSpacing="0.15em"
          textTransform="uppercase"
          mb={2}
          opacity={0.9}
        >
          Hint {step + 1}/{totalSteps}
        </Text>

        {visual && (
          <Box mb={3} maxW="240px" mx="auto" borderRadius="10px" overflow="hidden">
            {visual}
          </Box>
        )}

        <VStack align="start" spacing={1} mb={3}>
          <Text fontWeight="bold" fontSize="md" color="gray.100" lineHeight="short">
            {title}
          </Text>
          <Text fontSize="sm" color="gray.400" lineHeight="tall">
            {body}
          </Text>
        </VStack>

        <HStack justify="space-between" align="center">
          <HStack spacing={1.5}>
            {Array.from({ length: totalSteps }, (_, i) => (
              <Box
                key={i}
                w={i === step ? '14px' : '5px'}
                h="5px"
                rounded="full"
                bg={i === step ? 'blue.400' : 'gray.600'}
                transition="all 0.25s ease"
                cursor="pointer"
                onClick={() => onSelectStep(i)}
                _hover={{ bg: i === step ? 'blue.400' : 'gray.500' }}
              />
            ))}
          </HStack>
          <HStack spacing={1}>
            <Button
              size="xs"
              variant="ghost"
              color="gray.500"
              _hover={{ color: 'gray.300' }}
              onClick={onBack}
              visibility={step > 0 ? 'visible' : 'hidden'}
            >
              Back
            </Button>
            <Button size="xs" colorScheme="blue" px={3} onClick={isLast ? onDismiss : onNext}>
              {isLast ? 'Got it' : 'Next'}
            </Button>
          </HStack>
        </HStack>
      </Box>
    </Box>
  )
}
