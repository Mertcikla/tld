package mermaid

import (
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
)

func TestThemePartialOverride(t *testing.T) {
	theme, err := ParseTheme([]byte(`{"version":1,"nodes":{"modified":{"fill":"#123456","font-weight":""}},"labels":{"churn":false}}`))
	if err != nil {
		t.Fatal(err)
	}
	if theme.Nodes.Modified["fill"] != "#123456" || theme.Nodes.Modified["stroke"] != "#ecc94b" || theme.Labels.Churn || theme.Labels.Change {
		t.Fatalf("override did not merge: %+v", theme)
	}
	if strings.Contains(themeStyle(theme.Nodes.Modified), "font-weight") {
		t.Fatal("empty property was not removed")
	}
	if DefaultTheme().Nodes.Modified["fill"] != "#2d3748" {
		t.Fatal("default mutated")
	}
	if _, err := ParseTheme(defaultThemeJSON); err != nil {
		t.Fatalf("embedded theme invalid: %v", err)
	}
}

func TestThemeRejectsInvalidOverrides(t *testing.T) {
	for _, data := range []string{
		`null`, `[]`,
		`{"themeVariables":null}`, `{"themeVariables":{"unknown":"#fff"}}`,
		`{"themeVariables":{"darkMode":"true"}}`, `{"themeVariables":{"primaryColor":42}}`,
		`{"themeVariables":{"clusterBkg":null}}`, `{"themeVariables":{"primaryColor":"red"}}`,
		`{"themeVariables":{"fontFamily":"Arial\n---\nA-->B"}}`,
		`{"themeVariables":{"background":"url(https://example.com)"}}`,
		`{"baseTheme":"unknown"}`, `{"baseTheme":"dark\n---\nA-->B"}`, `{"baseTheme":null}`, `{"baseTheme":""}`,
		`{"version":2}`, `{"typo":true}`, `{"labels":{"typo":true}}`, `{"nodes":{"typo":{}}}`,
		`{"nodes":{"modified":{"opacity":"0.5"}}}`, `{"nodes":{"modified":{"fill":"red;class x y"}}}`,
		`{"nodes":{"modified":{"fill":"#fff\nA-->B"}}}`, `{"nodes":{"modified":{"fill":"url(https://example.com)"}}}`,
		`{"nodes":{"modified":{"font-family":"Arial, sans-serif"}}}`, `{"nodes":{"modified":{"font-weight":600}}}`,
		`{"labels":{"addedColor":"red"}}`, `{"labels":{"removedColor":"#fff';display:none"}}`,
		`{"labels":{"addedColor":null}}`, `{"labels":{"churn":null}}`, `{"nodes":{"modified":null}}`, `{"nodes":{"added":{"fill":null}}}`,
		`{"version":1} {}`, `{"version":1`,
	} {
		t.Run(data, func(t *testing.T) {
			if _, err := ParseTheme([]byte(data)); err == nil {
				t.Fatal("accepted invalid theme")
			}
		})
	}
	if _, err := ParseTheme([]byte(`{}`)); err != nil {
		t.Fatal(err)
	}
}

func TestThemedSceneAndNoTheme(t *testing.T) {
	scene := impact.PublicScene(sceneFixture(), false)
	before := proto.Clone(scene)
	theme, err := ParseTheme([]byte(`{"baseTheme":"redux-dark-color"}`))
	if err != nil {
		t.Fatal(err)
	}
	themed := ExportImpactScene(scene, SceneExportOptions{Theme: theme})
	if !strings.HasPrefix(themed, "---\nconfig:\n  theme: 'redux-dark-color'\n  look: classic\n  htmlLabels: true\n  themeCSS: '.edgeLabel .labelBkg { background: transparent !important; }'\n  themeVariables:\n") {
		t.Fatalf("missing base theme frontmatter: %s", themed)
	}
	for _, want := range []string{`el_51_7("svc/auth<br/><span style='color:#ecc94b!important'>+3</span> <span style='color:#fc8181!important'>−1</span>")`, "class el_51_7 tld_modified", "class el_51_8 tld_context", "classDef tld_modified", "font-weight:600", "linkStyle 0 color:#718096,stroke:#48bb78,stroke-width:2px"} {
		if !strings.Contains(themed, want) {
			t.Fatalf("missing %q:\n%s", want, themed)
		}
	}
	if strings.Contains(themed, "<br/>modified") || strings.Contains(themed, "(context)") {
		t.Fatal("redundant status labels")
	}
	parsed, err := Parse(themed)
	if err != nil || len(parsed.Warnings) != 0 {
		t.Fatalf("invalid themed Mermaid: %v %+v", err, parsed)
	}
	plain := ExportImpactScene(scene, SceneExportOptions{Theme: DefaultTheme(), NoTheme: true})
	for _, forbidden := range []string{"config:", "classDef", "\nclass ", "linkStyle", ":::"} {
		if strings.Contains(plain, forbidden) {
			t.Fatalf("unstyled output contains %s", forbidden)
		}
	}
	if !strings.Contains(plain, `svc/auth<br/>modified +3 −1`) || !strings.Contains(plain, `el_51_8("svc/db")`) {
		t.Fatalf("unstyled semantics lost: %s", plain)
	}
	if !proto.Equal(before, scene) {
		t.Fatal("theme altered scene")
	}
}

func TestThemedIsolatedGroupsDoNotMixChangeStates(t *testing.T) {
	fixture := sceneFixture()
	content := fixture.Views["51"]
	content.Connectors = nil
	content.Placements = append(content.Placements,
		sceneNode(9, 51, "gone.go", sceneOverlay(pb.ChangeKind_CHANGE_KIND_REMOVED, "gone.go", 0, 2)),
		sceneNode(10, 51, "more.go", sceneOverlay(pb.ChangeKind_CHANGE_KIND_MODIFIED, "more.go", 1, 1)))
	got := ExportImpactScene(impact.PublicScene(fixture, false), SceneExportOptions{Theme: DefaultTheme()})
	for _, want := range []string{
		`view_51_isolated_context("svc/db")`,
		`view_51_isolated_modified("svc/auth <span style='color:#ecc94b!important'>+3</span> <span style='color:#fc8181!important'>−1</span><br/>more.go <span style='color:#ecc94b!important'>+1</span> <span style='color:#fc8181!important'>−1</span>")`,
		`view_51_isolated_removed("gone.go <span style='color:#fc8181!important'>−2</span>")`,
		`class view_51_isolated_removed tld_removed`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q:\n%s", want, got)
		}
	}
}

func TestThemeLabelsAndUnknownCounts(t *testing.T) {
	theme, err := ParseTheme([]byte(`{"labels":{"change":true,"context":true,"churn":false}}`))
	if err != nil {
		t.Fatal(err)
	}
	fixture := sceneFixture()
	fixture.Views["51"].Placements[1].Overlay = &model.Overlay{Change: pb.ChangeKind_CHANGE_KIND_UNSPECIFIED}
	got := ExportImpactScene(impact.PublicScene(fixture, false), SceneExportOptions{Theme: theme})
	if !strings.Contains(got, "svc/auth<br/>modified") || !strings.Contains(got, "svc/db<br/>(context)") || strings.Contains(got, "+3") {
		t.Fatalf("label override ignored: %s", got)
	}
	fixture.Views["51"].Placements[0].Overlay.LinesAdded = nil
	got = ExportImpactScene(impact.PublicScene(fixture, false), SceneExportOptions{Theme: DefaultTheme()})
	if !strings.Contains(got, `el_51_7("svc/auth")`) {
		t.Fatalf("invented unknown counts: %s", got)
	}
}

func TestThemeVariablesFrontmatter(t *testing.T) {
	theme, err := ParseTheme([]byte(`{"themeVariables":{"fontFamily":"'Example Font', sans-serif","primaryColor":"#123456","darkMode":false}}`))
	if err != nil {
		t.Fatal(err)
	}
	output := ExportImpactScene(nil, SceneExportOptions{Theme: theme})
	parts := strings.Split(output, "---\n")
	if len(parts) != 3 {
		t.Fatalf("missing YAML frontmatter: %s", output)
	}
	var frontmatter struct {
		Config struct {
			Theme     string         `yaml:"theme"`
			Look      string         `yaml:"look"`
			Variables map[string]any `yaml:"themeVariables"`
		} `yaml:"config"`
	}
	if err := yaml.Unmarshal([]byte(parts[1]), &frontmatter); err != nil {
		t.Fatal(err)
	}
	if frontmatter.Config.Theme != "base" || frontmatter.Config.Look != "classic" {
		t.Fatalf("unexpected config: %+v", frontmatter.Config)
	}
	for key, want := range map[string]any{
		"primaryColor": "#123456", "darkMode": false,
		"fontFamily": "'Example Font', sans-serif",
		"background": "transparent", "clusterBkg": "transparent", "edgeLabelBackground": "transparent",
	} {
		if got := frontmatter.Config.Variables[key]; got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
	if DefaultTheme().ThemeVariables["primaryColor"] != "#2d3748" {
		t.Fatal("override mutated defaults")
	}
	if output != ExportImpactScene(nil, SceneExportOptions{Theme: theme}) {
		t.Fatal("nondeterministic frontmatter")
	}
}

func TestChurnLabelColorsOverride(t *testing.T) {
	theme, err := ParseTheme([]byte(`{"labels":{"addedColor":"#123456","removedColor":""}}`))
	if err != nil {
		t.Fatal(err)
	}
	got := ExportImpactScene(impact.PublicScene(sceneFixture(), false), SceneExportOptions{Theme: theme})
	if !strings.Contains(got, `svc/auth<br/><span style='color:#123456!important'>+3</span> −1`) {
		t.Fatalf("color override ignored: %s", got)
	}
}
