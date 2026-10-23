import React, { useRef } from 'react'
import { Box, IconButton, Input, InputGroup, InputLeftElement, InputRightElement } from '@chakra-ui/react'
import { CloseIcon, SearchIcon } from '@chakra-ui/icons'
import { editorControlStyles } from '../../styles/editorInteractions'

interface Props {
  query: string
  setQuery: (q: string) => void
  activeFilter: 'out' | 'in' | null
  onKeyDown?: (e: React.KeyboardEvent) => void
}

export const ViewSearch: React.FC<Props> = ({ query, setQuery, activeFilter, onKeyDown }) => {
  const inputRef = useRef<HTMLInputElement>(null)
  const label = activeFilter === 'out' ? 'Search parents' : activeFilter === 'in' ? 'Search children' : 'Search views'
  return (
    <Box className="panel-search-container">
      <InputGroup size="sm">
        <InputLeftElement pointerEvents="none">
          <SearchIcon color="gray.500" />
        </InputLeftElement>
        <Input
          ref={inputRef}
          aria-label={label}
          data-testid="view-explorer-search"
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
          onKeyDown={onKeyDown}
          pr={8}
        />
        <InputRightElement>
          <IconButton
            aria-label="Clear view search"
            icon={<CloseIcon boxSize="9px" />}
            size="xs"
            variant="ghost"
            color="gray.400"
            opacity={query ? 1 : 0}
            isDisabled={!query}
            aria-hidden={!query}
            _disabled={{ opacity: 0 }}
            tabIndex={query ? 0 : -1}
            pointerEvents={query ? 'auto' : 'none'}
            sx={editorControlStyles}
            onMouseDown={(event) => event.preventDefault()}
            onClick={() => {
              setQuery('')
              inputRef.current?.focus()
            }}
          />
        </InputRightElement>
      </InputGroup>
    </Box>
  )
}
