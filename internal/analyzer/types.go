package analyzer

// Symbol is a named declaration found in a source file.
type Symbol struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	FilePath    string `json:"file_path,omitempty"`
	Line        int    `json:"line"`
	EndLine     int    `json:"end_line,omitempty"`
	Parent      string `json:"parent,omitempty"`
	Technology  string `json:"technology,omitempty"`
	Description string `json:"description,omitempty"`
}

// Ref is a call-site reference to a named symbol found within a source file.
type Ref struct {
	Name       string `json:"name"`
	Kind       string `json:"kind,omitempty"`
	TargetPath string `json:"target_path,omitempty"`
	FilePath   string `json:"file_path,omitempty"`
	Line       int    `json:"line"`
	Column     int    `json:"column,omitempty"`
}

// Result holds the output of extracting symbols from one or more files.
type Result struct {
	Symbols []Symbol `json:"symbols"`
	Refs    []Ref    `json:"refs"`
}

func mergeResult(dst, src *Result) {
	if dst == nil || src == nil {
		return
	}
	dst.Symbols = append(dst.Symbols, src.Symbols...)
	dst.Refs = append(dst.Refs, src.Refs...)
}
