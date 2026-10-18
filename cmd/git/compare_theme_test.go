package git

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	"github.com/mertcikla/tld/v2/internal/mermaid"
)

func TestCompareThemeAcrossFormats(t *testing.T) {
	theme, err := mermaid.ParseTheme([]byte(`{"nodes":{"modified":{"fill":"#123456"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, auto := range []bool{false, true} {
		for _, markdown := range []bool{false, true} {
			for _, noTheme := range []bool{false, true} {
				opts := compareOptions{auto: auto, mermaid: !markdown, markdown: markdown, theme: theme, noTheme: noTheme, blastRadius: 1}
				diagram := autoCompareFixture(2, 1)
				autoCompareEdge(diagram, 1, 2)
				autoCompareEdge(diagram, 2, 3)
				diagram.Scene.Views["1"].Placements[0].Overlay = &model.Overlay{Change: pb.ChangeKind_CHANGE_KIND_MODIFIED}
				render := compareRenderer{ctx: context.Background(), opts: opts}
				out, size, err := render.build(diagram, 1)
				if auto {
					result, _, reason, e := selectAutoCompare(diagram, opts, render.build, autoFixtureLoader(diagram))
					if reason != "" {
						t.Fatal(reason)
					}
					out, size, err = result.text, result.size, e
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(out) != size {
					t.Fatal("styling omitted from byte budget")
				}
				if noTheme {
					if strings.Contains(out, "classDef") || strings.Contains(out, "linkStyle") {
						t.Fatalf("no-theme styled: %s", out)
					}
				} else if !strings.Contains(out, "fill:#123456") {
					t.Fatalf("custom theme not applied (auto=%t markdown=%t): %s", auto, markdown, out)
				}
			}
		}
	}
}

func TestCompareThemeValidationBeforeIndexing(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte(`{"nodes":{"modified":{"fill":"nope"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"--theme", bad}, "require --mermaid or --markdown"},
		{[]string{"--no-theme"}, "require --mermaid or --markdown"},
		{[]string{"--markdown", "--theme", bad, "--no-theme"}, "cannot be combined"},
		{[]string{"--markdown", "--theme", bad}, "invalid Mermaid theme"},
		{[]string{"--markdown", "--theme", bad + "missing"}, "read Mermaid theme"},
	} {
		cmd := newCompareCmd()
		cmd.SilenceUsage, cmd.SilenceErrors = true, true
		cmd.SetArgs(append([]string{"missing-repository", "main", "HEAD"}, test.args...))
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("got %v, want %s", err, test.want)
		}
	}
}
