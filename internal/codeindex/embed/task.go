package embed

import "fmt"

// Task is one retrieval task supported by the jina-code-embeddings family. The
// model is trained with asymmetric instruction prefixes: Query is prepended to
// the search text and Passage to every indexed chunk. The strings must match the
// model card verbatim, otherwise the model falls back to un-instructed
// embeddings and recall degrades.
type Task struct {
	Name    string
	Query   string
	Passage string
}

// Tasks is the instruction catalog published with jina-code-embeddings-1.5b (and
// the matching 0.5b model). Order is the model card order.
var Tasks = []Task{
	{
		Name:    "nl2code",
		Query:   "Find the most relevant code snippet given the following query:\n",
		Passage: "Candidate code snippet:\n",
	},
	{
		Name:    "code2code",
		Query:   "Find an equivalent code snippet given the following code snippet:\n",
		Passage: "Candidate code snippet:\n",
	},
	{
		Name:    "code2nl",
		Query:   "Find the most relevant comment given the following code snippet:\n",
		Passage: "Candidate comment:\n",
	},
	{
		Name:    "code2completion",
		Query:   "Find the most relevant completion given the following start of code snippet:\n",
		Passage: "Candidate completion:\n",
	},
	{
		Name:    "qa",
		Query:   "Find the most relevant answer given the following question:\n",
		Passage: "Candidate answer:\n",
	},
}

// LookupTask resolves a task by name. The empty name and unknown names report
// ok=false so callers can fall back to configured prefixes.
func LookupTask(name string) (Task, bool) {
	for _, t := range Tasks {
		if t.Name == name {
			return t, true
		}
	}
	return Task{}, false
}

// ResolveTask resolves a required task, returning a descriptive error for an
// empty or unknown name.
func ResolveTask(name string) (Task, error) {
	t, ok := LookupTask(name)
	if !ok {
		return Task{}, fmt.Errorf("unknown embedding task %q", name)
	}
	return t, nil
}
