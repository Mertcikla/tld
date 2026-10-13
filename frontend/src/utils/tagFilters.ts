export interface TagFilterState {
  selectedTags: string[]
  hiddenTags: string[]
}

export function toggleSelectedTag(selectedTags: string[], tag: string): TagFilterState {
  return {
    selectedTags: selectedTags.includes(tag) ? selectedTags.filter((item) => item !== tag) : [...selectedTags, tag],
    hiddenTags: [],
  }
}

export function toggleHiddenTags(hiddenTags: string[], tags: string[]): TagFilterState {
  const allHidden = tags.every((tag) => hiddenTags.includes(tag))
  return {
    selectedTags: [],
    hiddenTags: allHidden ? hiddenTags.filter((tag) => !tags.includes(tag)) : Array.from(new Set([...hiddenTags, ...tags])),
  }
}

export function resetTagFilters(): TagFilterState {
  return { selectedTags: [], hiddenTags: [] }
}
