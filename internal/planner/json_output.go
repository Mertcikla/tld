package planner

type JSONOutput struct {
	Command    string         `json:"command"`
	Status     string         `json:"status"`
	Summary    map[string]int `json:"summary,omitempty"`
	Items      []JSONItem     `json:"items,omitempty"`
	Errors     []string       `json:"errors,omitempty"`
	Warnings   []string       `json:"warnings,omitempty"`
	Retries    int            `json:"retries,omitempty"`
	DiffFiles  []JSONDiffFile `json:"diff_files,omitempty"`
	Extra      map[string]any `json:"extra,omitempty"`
	IsModified *bool          `json:"is_modified,omitempty"`
}

type JSONItem struct {
	Ref          string `json:"ref"`
	ResourceType string `json:"resource_type"`
	Action       string `json:"action"`
	Name         string `json:"name,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

type JSONDiffFile struct {
	Path  string   `json:"path"`
	Hunks []string `json:"hunks,omitempty"`
}
