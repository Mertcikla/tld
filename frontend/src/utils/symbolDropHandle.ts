/** Body drops face the other element; use the visible boundary when no origin is available. */
export function symbolDropHandle(target: Element, clientX: number, sourceNodeId?: string | null): string | null {
  const body = target.closest('[data-symbol-handle]')
  const id = body?.getAttribute('data-symbol-handle')
  if (!body || !id) return null
  const node = body.closest('.react-flow__node')
  if (sourceNodeId && node) {
    const origin = [...document.querySelectorAll('.react-flow__node')].find(candidate => candidate.getAttribute('data-id') === sourceNodeId)
    if (origin && origin !== node) {
      const source = origin.getBoundingClientRect(), destination = node.getBoundingClientRect()
      return id.replace(/^(left|right)-/, source.left + source.width / 2 < destination.left + destination.width / 2 ? 'left-' : 'right-')
    }
  }
  const key = body.getAttribute('data-symbol-key')
  const region = key ? [...(node?.querySelectorAll('.sub-symbol-region') ?? [])].find(region => region.getAttribute('data-symbol-key') === key) : undefined
  const bounds = (region ?? body).getBoundingClientRect()
  const viewport = node?.getBoundingClientRect()
  const left = Math.max(bounds.left, viewport?.left ?? bounds.left)
  const right = Math.min(bounds.right, viewport?.right ?? bounds.right)
  return id.replace(/^(left|right)-/, clientX <= (left + right) / 2 ? 'left-' : 'right-')
}
