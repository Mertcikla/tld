import type { ReactNode } from 'react'
import { Box, Flex, Text, FlexProps } from '@chakra-ui/react'
import { TYPE_COLORS } from '../types'

interface ElementBodyProps extends FlexProps {
  name: string
  nameContent?: ReactNode
  type: string
  technology?: string
  logoUrl?: string
  nameSize?: string
  nameNoOfLines?: number
  typeSize?: string
  techSize?: string
}

export const ElementBody = ({
  name,
  nameContent,
  type,
  technology,
  logoUrl,
  nameSize = 'sm',
  nameNoOfLines = 2,
  typeSize = '2xs',
  techSize = 'xs',
  children,
  ...props
}: ElementBodyProps) => {
  const color = TYPE_COLORS[type] ?? 'gray'

  const hasLogo = !!logoUrl

  return (
    <Flex
      flexDir={hasLogo ? 'row' : 'column'}
      align={hasLogo ? 'center' : 'center'}
      justify="center"
      p={3}
      gap={hasLogo ? 2 : 1}
      {...props}
    >
      {hasLogo && (
        <Flex
          w="38px"
          h="38px"
          align="center"
          justify="center"
          flexShrink={0}
        >
          <Box
            as="img"
            src={logoUrl}
            alt="technology icon"
            maxW="100%"
            maxH="100%"
            objectFit="contain"
          />
        </Flex>
      )}

      <Flex flexDir="column" align={hasLogo ? 'flex-start' : 'center'} justify="center" flex={1} minW={0}>
        {nameContent ?? (
          <Text
            fontWeight="semibold"
            fontSize={nameSize}
            noOfLines={nameNoOfLines}
            textAlign={hasLogo ? 'left' : 'center'}
            color="gray.100"
            lineHeight={1.15}
          >
            {name}
          </Text>
        )}
        {!!type && (
          <Text
            fontSize={typeSize}
            color={`${color}.400`}
            textAlign={hasLogo ? 'left' : 'center'}
            textTransform="uppercase"
            letterSpacing="0.06em"
            lineHeight={1.1}
          >
            {type}
          </Text>
        )}
        {technology && (
          <Text fontSize={techSize} color="gray.500" textAlign={hasLogo ? 'left' : 'center'} lineHeight={1.1}>
            [{technology}]
          </Text>
        )}
        {children}
      </Flex>
    </Flex>
  )
}
