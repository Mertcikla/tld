import React from 'react'
import { Box } from '@chakra-ui/react'

export function KbdHint({ children }: { children: string }) {
  return (
    <Box
      as="span"
      display="inline-flex"
      alignItems="center"
      justifyContent="center"
      px={1.5}
      py={0.5}
      bg="whiteAlpha.300"
      rounded="sm"
      fontSize="8px"
      fontWeight="bold"
      color="whiteAlpha.900"
      flexShrink={0}
      ml={2}
    >
      {children}
    </Box>
  )
}
