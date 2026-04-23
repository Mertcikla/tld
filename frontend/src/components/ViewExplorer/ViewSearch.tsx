import React from 'react'
import { Box, Input, InputGroup, InputLeftElement } from '@chakra-ui/react'
import { SearchIcon } from '@chakra-ui/icons'

interface Props {
  query: string
  setQuery: (q: string) => void
  activeFilter: 'out' | 'in' | null
}

export const ViewSearch: React.FC<Props> = ({ query, setQuery, activeFilter }) => {
  return (
    <Box className="panel-search-container">
      <InputGroup size="sm">
        <InputLeftElement pointerEvents="none">
          <SearchIcon color="gray.500" />
        </InputLeftElement>
        <Input
          className="panel-search-input"
          placeholder={
            activeFilter === 'out'
              ? 'Search parents…'
              : activeFilter === 'in'
                ? 'Search children…'
                : 'Search views…'
          }
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </InputGroup>
    </Box>
  )
}
