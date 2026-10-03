import { useMemo } from 'react'
import { Box, Text } from '@chakra-ui/react'
import type { RepositoryImpact } from '../api/client'
import OpenInEditorButton from './OpenInEditorButton'

const colors = { added: 'green.300', removed: 'red.300', modified: 'yellow.300' }


export default function RepositorySymbols({ repositoryRoot, path, diagram, inline = false }: {
  repositoryRoot: string; path: string; diagram: RepositoryImpact | null; inline?: boolean
}) {
  const symbols = useMemo(() => {
    const changes = diagram?.diff.factDetails ?? {
      added: diagram?.nodes.flatMap((node) => node.symbols.added) ?? [],
      removed: diagram?.nodes.flatMap((node) => node.symbols.removed) ?? [],
      modified: diagram?.nodes.flatMap((node) => node.symbols.modified) ?? [],
    }
    const changed = (['added', 'removed', 'modified'] as const).flatMap((kind) =>
      (changes?.[kind] ?? []).filter((fact) => fact.anchor?.path && (!path || fact.anchor.path === path)).map((fact) => ({ fact, kind })))
    return changed
  }, [diagram, path])
  const groups = useMemo(() => {
    const grouped = new Map<string, typeof symbols>()
    for (const symbol of symbols) {
      const filePath = symbol.fact.anchor?.path || path
      const group = grouped.get(filePath) ?? []
      group.push(symbol)
      grouped.set(filePath, group)
    }
    return [...grouped.entries()].sort(([a], [b]) => a.localeCompare(b))
  }, [symbols, path])
  return (
    <Box role={inline ? undefined : 'tabpanel'} aria-label={inline ? undefined : 'Symbols'} px={inline ? 0 : 3} pb={inline ? 0 : 3} overflowY={inline ? undefined : 'auto'} maxH={inline ? undefined : { base: '220px', lg: 'none' }} data-testid="repository-symbols">

        {!symbols.length && <Text fontSize="xs" color="gray.400">{path ? 'No changed symbols in this file.' : 'No changed symbols in this comparison.'}</Text>}
        {groups.map(([filePath, fileSymbols]) => <Box key={filePath} mb={inline ? 0 : 4} data-symbol-file={filePath}>
          {!inline && <Text fontSize="xs" color="gray.400" wordBreak="break-all" mb={3}>{filePath}</Text>}
          {fileSymbols.map(({ fact, kind }) => <Box key={`${kind}:${fact.id}`} mb={inline ? 0 : 3} py={inline ? 1 : 0} position="relative" data-symbol-change={kind} tabIndex={0}
          _focusVisible={{ outline: '2px solid var(--accent)', outlineOffset: '2px' }}
          sx={{ '&:hover .symbol-editor-action, &:focus-within .symbol-editor-action': { display: 'block' } }}>
          <Text fontSize="xs" color={colors[kind]} overflowWrap="anywhere">{fact.name || fact.qualifiedName}{!!fact.anchor?.startLine && <Text as="span" color="gray.500"> · L{fact.anchor.startLine}</Text>}</Text>
          <Box className="symbol-editor-action" display="none" position="absolute" right={0} top="-3px" textAlign="right" bg="var(--bg-canvas)"><OpenInEditorButton borderless repo={repositoryRoot} filePath={filePath} line={fact.anchor?.startLine || null} /></Box>
          </Box>)}
        </Box>)}
    </Box>
  )
}
