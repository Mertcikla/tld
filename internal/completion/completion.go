// Package completion provides safe, fast cobra shell-completion helpers that
// resolve candidates from the local workspace YAML files and, optionally, from
// the remote API.
//
// All helpers are designed to run on every TAB press: they never panic, never
// write to stderr, and never block the shell. Remote lookups are gated behind
// TLD_COMPLETION_REMOTE=1 and use a short deadline so an offline or
// unauthenticated environment never stalls completion.
package completion

import (
	"context"
	"sort"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/internal/client"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/spf13/cobra"
)

const (
	remoteEnvVar   = "TLD_COMPLETION_REMOTE"
	remoteTimeout  = 300 * time.Millisecond
	remotePageSize = 500
)

func loadWS(wdir *string) *workspace.Workspace {
	if wdir == nil {
		return nil
	}
	ws, err := workspace.Load(*wdir)
	if err != nil {
		return nil
	}
	return ws
}

func remoteEnabled() bool {
	state, err := workspace.LoadGlobalConfigStateNoRepair()
	if err != nil {
		return false
	}
	return state.Config.Completion.Remote
}

// remoteElements fetches elements from the API with a short deadline. Any
// failure (disabled, missing creds, network error, timeout) returns nil.
func remoteElements(ws *workspace.Workspace) []*diagv1.Element {
	if !remoteEnabled() || ws == nil {
		return nil
	}
	if ws.Config.ServerURL == "" || ws.Config.APIKey == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteTimeout)
	defer cancel()
	c := client.New(ws.Config.ServerURL, ws.Config.APIKey, false)
	resp, err := c.ListElements(ctx, connect.NewRequest(&diagv1.ListElementsRequest{Limit: remotePageSize}))
	if err != nil || resp == nil {
		return nil
	}
	return resp.Msg.Elements
}

// ElementRefs returns sorted unique element refs from local yaml and,
// optionally, the remote API.
func ElementRefs(wdir *string) (out []string, dir cobra.ShellCompDirective) {
	dir = cobra.ShellCompDirectiveNoFileComp
	defer func() { _ = recover() }()
	ws := loadWS(wdir)
	seen := map[string]struct{}{}
	if ws != nil {
		for ref := range ws.Elements {
			seen[ref] = struct{}{}
		}
	}
	for _, e := range remoteElements(ws) {
		if e != nil && e.Ref != "" {
			seen[e.Ref] = struct{}{}
		}
	}
	out = make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Strings(out)
	return
}

// ElementRefsWithNames returns element refs tagged with their display name in
// cobra's "value\tdescription" format so zsh/fish show the human name.
func ElementRefsWithNames(wdir *string) (out []string, dir cobra.ShellCompDirective) {
	dir = cobra.ShellCompDirectiveNoFileComp
	defer func() { _ = recover() }()
	ws := loadWS(wdir)
	labels := map[string]string{}
	if ws != nil {
		for ref, el := range ws.Elements {
			name := ""
			if el != nil {
				name = el.Name
			}
			labels[ref] = name
		}
	}
	for _, e := range remoteElements(ws) {
		if e == nil || e.Ref == "" {
			continue
		}
		if _, ok := labels[e.Ref]; !ok {
			labels[e.Ref] = e.Name
		}
	}
	refs := make([]string, 0, len(labels))
	for r := range labels {
		refs = append(refs, r)
	}
	sort.Strings(refs)
	out = make([]string, 0, len(refs))
	for _, r := range refs {
		if name := labels[r]; name != "" {
			out = append(out, r+"\t"+name)
		} else {
			out = append(out, r)
		}
	}
	return
}

// ConnectorKeys returns "view:from:to" keys for every local connector.
func ConnectorKeys(wdir *string) (out []string, dir cobra.ShellCompDirective) {
	dir = cobra.ShellCompDirectiveNoFileComp
	defer func() { _ = recover() }()
	ws := loadWS(wdir)
	if ws == nil {
		return
	}
	out = make([]string, 0, len(ws.Connectors))
	for key := range ws.Connectors {
		out = append(out, key)
	}
	sort.Strings(out)
	return
}

// ViewRefs returns refs of elements that host a view, plus the synthetic
// "root" view.
func ViewRefs(wdir *string) (out []string, dir cobra.ShellCompDirective) {
	dir = cobra.ShellCompDirectiveNoFileComp
	defer func() { _ = recover() }()
	ws := loadWS(wdir)
	seen := map[string]struct{}{"root": {}}
	if ws != nil {
		for ref, el := range ws.Elements {
			if el != nil && el.HasView {
				seen[ref] = struct{}{}
			}
		}
	}
	out = make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Strings(out)
	return
}

// ParentRefs returns every element ref plus "root" for --parent completion.
func ParentRefs(wdir *string) (out []string, dir cobra.ShellCompDirective) {
	dir = cobra.ShellCompDirectiveNoFileComp
	defer func() { _ = recover() }()
	refs, _ := ElementRefs(wdir)
	seen := map[string]struct{}{"root": {}}
	for _, r := range refs {
		seen[r] = struct{}{}
	}
	out = make([]string, 0, len(seen))
	for r := range seen {
		out = append(out, r)
	}
	sort.Strings(out)
	return
}

// ElementFields is the static set of fields accepted by `update element`.
func ElementFields() []string {
	return []string{"name", "kind", "description", "technology", "url", "owner", "logo_url", "repo", "branch", "language", "file_path", "view_label"}
}

// ConnectorFields is the static set of fields accepted by `update connector`.
func ConnectorFields() []string {
	return []string{"label", "description", "relationship", "direction", "url", "source_handle", "target_handle"}
}

// ElementKinds are the built-in element kinds offered for --kind.
func ElementKinds() []string {
	return []string{"service", "database", "person", "system", "container", "component", "external"}
}

// ConnectorDirections are the valid values for --direction.
func ConnectorDirections() []string {
	return []string{"forward", "backward", "both", "none"}
}
