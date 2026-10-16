package cmdutil

type JSONOutput struct {
	Command  string         `json:"command"`
	Status   string         `json:"status"`
	DryRun   bool           `json:"dry_run"`
	Summary  map[string]int `json:"summary,omitempty"`
	Items    []JSONItem     `json:"items,omitempty"`
	Errors   []string       `json:"errors,omitempty"`
	Error    *CommandError  `json:"error,omitempty"`
	Warnings []string       `json:"warnings,omitempty"`
	Extra    map[string]any `json:"extra,omitempty"`
}

type JSONItem struct {
	Ref          string `json:"ref"`
	ResourceType string `json:"resource_type"`
	Action       string `json:"action"`
	Name         string `json:"name,omitempty"`
	Reason       string `json:"reason,omitempty"`
	ID           int32  `json:"id,omitempty"`
}
