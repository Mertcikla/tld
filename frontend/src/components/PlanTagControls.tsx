import { Button, Popover, PopoverBody, PopoverContent, PopoverTrigger, Portal } from '@chakra-ui/react'
import { TagFilterRow } from './TagFilterControls'

const planTags = ['plan:create', 'plan:modify', 'plan:remove', 'plan:include', 'plan:preserve', 'plan:context']

export default function PlanTagControls({ hiddenTags, selectedTags, tagColors, tagCounts, tagElementCounts, tagConnectorCounts, onSelect, onToggleVisibility, onHighlight }: {
  hiddenTags: string[]
  selectedTags: string[]
  tagColors: Record<string, { color?: string }>
  tagCounts: Record<string, number>
  tagElementCounts: Record<string, number>
  tagConnectorCounts: Record<string, number>
  onSelect: (tag: string) => void
  onToggleVisibility: (tag: string) => void
  onHighlight: (tags: string[] | null, color: string | null) => void
}) {
  const clearHighlight = () => onHighlight(null, null)
  return (
    <Popover placement="bottom" isLazy onClose={clearHighlight}>
      <PopoverTrigger><Button size="xs" variant="ghost">Plan tags</Button></PopoverTrigger>
      <Portal>
        <PopoverContent w="240px" maxW="calc(100vw - 32px)" bg="var(--bg-panel)" borderColor="whiteAlpha.100" onMouseLeave={clearHighlight}>
          <PopoverBody p={2}>
            {planTags.map(tag => (
              <TagFilterRow
                key={tag}
                tag={tag}
                count={tagCounts[tag] ?? 0}
                elementCount={tagElementCounts[tag] ?? 0}
                connectorCount={tagConnectorCounts[tag] ?? 0}
                color={tagColors[tag]?.color}
                hidden={hiddenTags.includes(tag)}
                selected={selectedTags.includes(tag)}
                onSelect={onSelect}
                onHover={() => onHighlight([tag], tagColors[tag]?.color || '')}
                onToggleVisibility={onToggleVisibility}
              />
            ))}
          </PopoverBody>
        </PopoverContent>
      </Portal>
    </Popover>
  )
}
