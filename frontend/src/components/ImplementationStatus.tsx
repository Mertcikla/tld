import { Box, Button, Popover, PopoverBody, PopoverContent, PopoverTrigger, Portal, Text } from '@chakra-ui/react'
import { useNavigate } from 'react-router-dom'
import { usePlanImplementation } from '../hooks/usePlanImplementation'
import PlanImplementationReview from './PlanImplementationReview'

/** Shared navigation surface: recovery remains available outside Watch. */
export default function ImplementationStatus() {
  const implementation = usePlanImplementation('', '', true, false)
  const navigate = useNavigate()
  const state = implementation.state
  if (!state?.locked) return null
  return <Popover placement="bottom-end" isLazy>
    <PopoverTrigger><Button size="xs" colorScheme="blue" flexShrink={0} maxW={{ base: '120px', md: '240px' }} overflow="hidden" textOverflow="ellipsis" display="block">Implementing · {implementation.plan?.name || 'Plan'}</Button></PopoverTrigger>
    <Portal><PopoverContent w="480px" maxW="calc(100vw - 24px)"><PopoverBody>
      <Text fontSize="sm">The workspace is staging edits for this implementation.</Text>
      <Button size="sm" variant="ghost" onClick={() => navigate(`/repositories?repo=${encodeURIComponent(state.repositoryId)}&mode=plans&plan=${encodeURIComponent(state.planId)}`)}>Open plan and review</Button>
      <Box><PlanImplementationReview implementation={implementation} /></Box>
    </PopoverBody></PopoverContent></Portal>
  </Popover>
}
