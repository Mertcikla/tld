package parser

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"gopkg.in/yaml.v3"
)

// helmFacts renders a chart with local default values. The source marker in
// Helm's output identifies the original template; each fact is rebound to the
// line that declares its kind or property there.
func helmFacts(root, chart string) []Fact {
	if _, e := exec.LookPath("helm"); e != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, e := exec.CommandContext(ctx, "helm", "template", "c4q", chart, "--include-crds").Output()
	if e != nil {
		return nil
	}
	var out []Fact
	for _, segment := range bytes.Split(raw, []byte("\n---\n")) {
		source := ""
		for _, line := range strings.Split(string(segment), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "# Source: ") {
				source = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "# Source: "))
				break
			}
		}
		at := strings.Index(source, "/templates/")
		if at < 0 {
			continue
		}
		original := filepath.Join(chart, filepath.FromSlash(source[at+1:]))
		body, e := os.ReadFile(original)
		if e != nil {
			continue
		}
		var doc yaml.Node
		if yaml.Unmarshal(segment, &doc) != nil || len(doc.Content) == 0 {
			continue
		}
		rel, e := filepath.Rel(root, original)
		if e != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		kind := scalar(field(doc.Content[0], "kind"))
		if kind == "" {
			continue
		}
		for _, f := range kubernetesFacts(doc.Content[0], rel, region(rel), kind) {
			out = append(out, rebindHelmFact(f, body))
		}
	}
	return out
}

func rebindHelmFact(f Fact, original []byte) Fact {
	key := "kind:"
	switch f.Kind {
	case pb.FactKind_FACT_KIND_DEPENDENCY, pb.FactKind_FACT_KIND_DATASTORE, pb.FactKind_FACT_KIND_QUEUE:
		key = "image:"
	case pb.FactKind_FACT_KIND_CONFIG_REF:
		key = f.Object + ":"
	}
	line := 1
	text := ""
	for i, raw := range bytes.Split(original, []byte("\n")) {
		value := strings.TrimSpace(string(raw))
		if strings.HasPrefix(value, key) || strings.HasPrefix(value, "- "+key) {
			line, text = i+1, value
			break
		}
	}
	if text == "" {
		text = strings.TrimSpace(strings.Split(string(original), "\n")[0])
	}
	f.Line = line
	f.Text = text
	f.Extractor = "helm"
	return f
}
