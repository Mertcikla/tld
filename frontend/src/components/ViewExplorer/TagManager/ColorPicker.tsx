import React from 'react'
import {
  Box,
  SimpleGrid,
  PopoverContent,
  PopoverBody,
  PopoverArrow,
} from '@chakra-ui/react'
import { SWATCH_COLORS } from '../utils'

interface Props {
  onSelect: (color: string) => void
  onClose: () => void
}

export const ColorPicker: React.FC<Props> = ({ onSelect, onClose }) => {
  return (
    <PopoverContent
      bg="gray.800"
      border="1px solid"
      borderColor="whiteAlpha.200"
      shadow="2xl"
      w="160px"
      zIndex={2000}
    >
      <PopoverArrow bg="gray.800" />
      <PopoverBody p={2}>
        <SimpleGrid columns={5} spacing={2}>
          {SWATCH_COLORS.map((c) => (
            <Box
              key={c}
              w="20px"
              h="20px"
              bg={c}
              rounded="full"
              cursor="pointer"
              _hover={{ transform: 'scale(1.1)', boxShadow: '0 0 0 2px white' }}
              onClick={() => {
                onSelect(c)
                onClose()
              }}
              transition="all 0.1s"
            />
          ))}
        </SimpleGrid>
      </PopoverBody>
    </PopoverContent>
  )
}
