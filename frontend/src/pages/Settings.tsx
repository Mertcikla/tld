import { Box, Flex, Text, VStack } from '@chakra-ui/react'
import { useEffect } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { useSetHeader } from '../components/HeaderContext'



const DEFAULT_NAV_ITEMS = [
  { label: 'Appearance', path: '/settings/appearance' },
]

export interface SettingsProps {
  extraNavItems?: Array<{ label: string; path: string }>
}

export default function Settings({ extraNavItems = [] }: SettingsProps) {
  const navItems = [...extraNavItems, ...DEFAULT_NAV_ITEMS]
  const navigate = useNavigate()
  const location = useLocation()
  const setHeader = useSetHeader()

  // Clear any page-specific header when on settings
  useEffect(() => {
    setHeader(null)
    return () => setHeader(null)
  }, [setHeader])



  return (
    <Flex direction="column" h="100%">
      <Flex flex={1} overflow="hidden" direction={{ base: 'column', md: 'row' }}>
        {/* Sidebar (hidden on small screens) */}
        <VStack
          w={{ base: '0', md: '200px' }}
          display={{ base: 'none', md: 'flex' }}
          flexShrink={0}
          bg="var(--bg-panel)"
          borderRight="1px solid"
          borderColor="var(--border-main)"
          py={4}
          px={2}
          spacing={1}
          align="stretch"
        >
          <Text
            fontSize="xs"
            fontWeight="bold"
            color="gray.500"
            textTransform="uppercase"
            px={2}
            mb={2}
          >
            Settings
          </Text>
          {navItems.map((item) => {
            const active = location.pathname === item.path
            return (
              <Box
                key={item.path}
                as="button"
                px={3}
                py={1.5}
                fontSize="sm"
                textAlign="left"
                borderRadius="md"
                color={active ? 'gray.100' : 'gray.400'}
                bg={active ? 'whiteAlpha.100' : 'transparent'}
                _hover={{ bg: 'whiteAlpha.50', color: 'gray.200' }}
                onClick={() => navigate(item.path)}
              >
                {item.label}
              </Box>
            )
          })}
        </VStack>

        {/* Main content - allow scrolling on small screens */}
        <Box
          flex={1}
          minH={0}
          overflowY="auto"
          p={6}
          pb={{ base: 'calc(var(--bottomnav-container-h) + env(safe-area-inset-bottom, 0px) + 24px)', sm: 6 }}
        >
          <Outlet />
        </Box>
      </Flex>
    </Flex>
  )
}
