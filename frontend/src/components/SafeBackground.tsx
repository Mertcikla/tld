import { type ComponentPropsWithoutRef } from 'react'
import { Background } from 'reactflow'
import { useStore } from 'reactflow'

/**
 * Guards React Flow's Background against NaN zoom.
 *
 * In @reactflow/background, `scaledSize = patternSize * transform[2]` has no
 * NaN guard (unlike scaledGap which uses `|| 1`). When d3-zoom produces a NaN
 * transform during fitView on a zero-size container, scaledSize becomes NaN,
 * propagating to patternOffset and the circle cx/cy attributes.
 *
 * This wrapper simply skips rendering until zoom is a positive finite number.
 */
export function SafeBackground(props: ComponentPropsWithoutRef<typeof Background>) {
  const [x, y, zoom] = useStore((s) => s.transform)
  if (!isFinite(zoom) || zoom <= 0 || !isFinite(x) || !isFinite(y)) return null
  return <Background {...props} />
}
