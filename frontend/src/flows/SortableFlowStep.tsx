import { useEffect, useRef, type ReactNode } from 'react'
import { Box, IconButton, Tooltip } from '@chakra-ui/react'
import { DragHandleIcon } from '@chakra-ui/icons'
import { Reorder, useDragControls } from 'framer-motion'

export function SortableFlowList<T extends { id: string }>({ items, onChange, disabled, kind = 'step', children }: {
  items: T[]; onChange: (items: T[]) => void; disabled: boolean; kind?: 'step' | 'path'; children: (item: T, index: number, handle: ReactNode) => ReactNode
}) {
  function move(index: number, delta: number) {
    if (disabled || index + delta < 0 || index + delta >= items.length) return
    const next = [...items]
    const [item] = next.splice(index, 1)
    next.splice(index + delta, 0, item)
    onChange(next)
  }
  return <Reorder.Group as="div" axis="y" values={items} onReorder={onChange} style={{ display: 'flex', flexDirection: 'column', gap: 12, paddingLeft: 26 }}>
    {items.map((item, index) => <SortableFlowItem key={item.id} item={item} label={`Reorder ${kind} ${index + 1}`} kind={kind} disabled={disabled || items.length < 2} onMove={delta => move(index, delta)}>
      {handle => children(item, index, handle)}
    </SortableFlowItem>)}
  </Reorder.Group>
}

function SortableFlowItem<T extends { id: string }>({ item, label, kind, disabled, onMove, children }: {
  item: T; label: string; kind: 'step' | 'path'; disabled: boolean; onMove: (delta: number) => void; children: (handle: ReactNode) => ReactNode
}) {
  const controls = useDragControls()
  const row = useRef<HTMLDivElement>(null)
  const scrollFrame = useRef(0)
  const pointerY = useRef(0)
  const stopScroll = () => { if (scrollFrame.current) cancelAnimationFrame(scrollFrame.current); scrollFrame.current = 0 }
  useEffect(() => () => { if (scrollFrame.current) cancelAnimationFrame(scrollFrame.current) }, [])

  function startScroll() {
    const body = row.current?.closest<HTMLElement>('[data-flow-scroll]')
    if (!body) return
    const scroll = () => {
      const { top, bottom } = body.getBoundingClientRect()
      const edge = 48
      const distance = pointerY.current < top + edge ? pointerY.current - top - edge : pointerY.current > bottom - edge ? pointerY.current - bottom + edge : 0
      body.scrollTop += Math.max(-12, Math.min(12, distance / 4))
      scrollFrame.current = requestAnimationFrame(scroll)
    }
    stopScroll()
    scrollFrame.current = requestAnimationFrame(scroll)
  }

  const handle = <Box position="absolute" left="-27px" top={0} bottom={0} display="flex" alignItems="center">
      <Tooltip label="Drag to reorder · ↑ / ↓ to move" openDelay={300} closeOnPointerDown closeOnScroll>
        <Box as="span" display="inline-flex">
          <IconButton aria-label={label} icon={<DragHandleIcon />} size="xs" w="24px" h="32px" minW="24px" variant="ghost"
            color="gray.400" cursor={disabled ? 'default' : 'grab'} _active={{ cursor: 'grabbing' }} isDisabled={disabled} sx={{ touchAction: 'none' }}
            onPointerDown={event => { if (!disabled) controls.start(event) }}
            onKeyDown={event => {
              if (!disabled && (event.key === 'ArrowUp' || event.key === 'ArrowDown')) {
                event.preventDefault(); event.stopPropagation(); onMove(event.key === 'ArrowUp' ? -1 : 1)
              }
            }} />
        </Box>
      </Tooltip>
    </Box>

  return <Reorder.Item as="div" ref={row} value={item} layout="position" transition={{ layout: { duration: 0 } }} dragListener={false} dragControls={controls} dragMomentum={false}
    data-flow-step-id={kind === 'step' ? item.id : undefined} data-flow-path-id={kind === 'path' ? item.id : undefined} style={{ position: 'relative' }}
    onDragStart={(_, info) => { pointerY.current = info.point.y; startScroll() }}
    onDrag={(_, info) => { pointerY.current = info.point.y }} onDragEnd={stopScroll}>
    {children(handle)}
  </Reorder.Item>
}
