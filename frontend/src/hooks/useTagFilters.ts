import { useCallback, useState } from 'react'
import { resetTagFilters, toggleHiddenTags, toggleSelectedTag } from '../utils/tagFilters'

export function useTagFilters() {
  const [filters, setFilters] = useState(resetTagFilters)
  const toggleTagSelection = useCallback((tag: string) => {
    setFilters((prev) => toggleSelectedTag(prev.selectedTags, tag))
  }, [])
  const toggleTagsVisibility = useCallback((tags: string[]) => {
    if (tags.length === 0) return
    setFilters((prev) => toggleHiddenTags(prev.hiddenTags, tags))
  }, [])
  const toggleTagVisibility = useCallback((tag: string) => toggleTagsVisibility([tag]), [toggleTagsVisibility])
  const resetFilters = useCallback(() => setFilters(resetTagFilters()), [])
  return { ...filters, toggleTagSelection, toggleTagVisibility, toggleTagsVisibility, resetFilters }
}
