package cmdutil

import (
	"maps"
	"reflect"
	"slices"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestConvertExportResponseAllocatesStableElementRefs(t *testing.T) {
	for _, tc := range []struct {
		name     string
		base     *workspace.Workspace
		elements []*diagv1.Element
		want     map[int32]string
	}{
		{
			name: "slug collision", base: &workspace.Workspace{},
			elements: []*diagv1.Element{{Id: 2, Name: "API-Service"}, {Id: 1, Name: "API Service"}},
			want:     map[int32]string{1: "api-service", 2: "api-service-2"},
		},
		{
			name: "identical names", base: &workspace.Workspace{},
			elements: []*diagv1.Element{{Id: 3, Name: "Run"}, {Id: 2, Name: "Run"}, {Id: 1, Name: "Run"}},
			want:     map[int32]string{1: "run", 2: "run-2", 3: "run-3"},
		},
		{
			name: "reserve local and cached refs",
			base: &workspace.Workspace{
				Elements: map[string]*workspace.Element{"api-service": {Name: "Local API"}, "api-service-1": {Name: "Local Worker"}},
				Meta:     &workspace.Meta{Elements: map[string]*workspace.ResourceMetadata{"api-service-2": {ID: 10}}},
			},
			elements: []*diagv1.Element{{Id: 1, Name: "API Service"}, {Id: 2, Name: "API Service"}, {Id: 10, Name: "Renamed API"}},
			want:     map[int32]string{1: "api-service-1-2", 2: "api-service-2-2", 10: "api-service-2"},
		},
		{
			name:     "reserve metadata without a server ID",
			base:     &workspace.Workspace{Meta: &workspace.Meta{Elements: map[string]*workspace.ResourceMetadata{"api-service": {}}}},
			elements: []*diagv1.Element{{Id: 1, Name: "API Service"}},
			want:     map[int32]string{1: "api-service-1"},
		},
		{
			name: "suffix does not steal another natural ref", base: &workspace.Workspace{},
			elements: []*diagv1.Element{{Id: 1, Name: "API Service"}, {Id: 2, Name: "API-Service"}, {Id: 3, Name: "API Service 2"}},
			want:     map[int32]string{1: "api-service", 2: "api-service-2-2", 3: "api-service-2"},
		},
		{
			name: "empty slug fallback also collides", base: &workspace.Workspace{},
			elements: []*diagv1.Element{{Id: 1, Name: "!!!"}, {Id: 2, Name: "Element 1"}},
			want:     map[int32]string{1: "element-1", 2: "element-1-2"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertRefs := func(base *workspace.Workspace, elements []*diagv1.Element) *workspace.Workspace {
				t.Helper()
				original := slices.Clone(elements)
				got := ConvertExportResponse(base, &diagv1.ExportOrganizationResponse{Elements: elements})
				if !slices.Equal(elements, original) {
					t.Fatal("conversion reordered the export response")
				}
				refs := make(map[int32]string)
				for ref, meta := range got.Meta.Elements {
					refs[int32(meta.ID)] = ref
				}
				if len(got.Elements) != len(elements) || !maps.Equal(refs, tc.want) {
					t.Fatalf("element refs = %v (%d elements), want %v", refs, len(got.Elements), tc.want)
				}
				return got
			}
			got := assertRefs(tc.base, tc.elements)
			reversed := slices.Clone(tc.elements)
			slices.Reverse(reversed)
			assertRefs(tc.base, reversed)
			// Cached identities retain their refs even when a name changes.
			renamed := make([]*diagv1.Element, 0, len(reversed))
			for _, element := range reversed {
				renamed = append(renamed, &diagv1.Element{Id: element.Id, Name: "Renamed"})
			}
			assertRefs(got, renamed)
		})
	}
}

func TestConvertExportResponseCollisionRoundTrip(t *testing.T) {
	t.Setenv("TLD_CONFIG_DIR", t.TempDir())
	base := &workspace.Workspace{Dir: t.TempDir()}
	if err := workspace.WriteLockFile(base.Dir, &workspace.LockFile{Version: "v1"}); err != nil {
		t.Fatal(err)
	}
	msg := &diagv1.ExportOrganizationResponse{
		Elements: []*diagv1.Element{{Id: 1, Name: "API Service"}, {Id: 2, Name: "API-Service"}, {Id: 3, Name: "API Service"}},
		Views: []*diagv1.View{
			{Id: 100, Name: "First API", OwnerElementId: new(int32(1))},
			{Id: 200, Name: "Second API", OwnerElementId: new(int32(2))},
			{Id: 300, Name: "Landscape"},
		},
		Placements: []*diagv1.ElementPlacement{
			{ElementId: 1, ViewId: 300, PositionX: 1, PositionY: 2},
			{ElementId: 2, ViewId: 100, PositionX: 3, PositionY: 4},
			{ElementId: 3, ViewId: 200, PositionX: 5, PositionY: 6},
			{ElementId: 3, ViewId: 300, PositionX: 7, PositionY: 8},
		},
		Connectors: []*diagv1.Connector{
			{Id: 50, ViewId: 100, SourceElementId: 1, TargetElementId: 3, Label: new("calls")},
			{Id: 51, ViewId: 100, SourceElementId: 2, TargetElementId: 3, Label: new("calls")},
		},
	}
	got := ConvertExportResponse(base, msg)
	assertIdentities := func(ws *workspace.Workspace) {
		t.Helper()
		if len(ws.Elements) != 3 || len(ws.Meta.Elements) != 3 || len(ws.Connectors) != 2 || len(ws.Meta.Connectors) != 2 || len(ws.Meta.Views) != 2 {
			t.Fatalf("resource counts changed: %#v", ws)
		}
		for i, ref := range []string{"api-service", "api-service-2", "api-service-3"} {
			element := ws.Elements[ref]
			name := []string{"API Service", "API-Service", "API Service"}[i]
			if element == nil || element.Name != name || ws.Meta.Elements[ref].ID != workspace.ResourceID(i+1) {
				t.Fatalf("element identity lost for %s", ref)
			}
			var want []workspace.ViewPlacement
			for _, p := range msg.Placements {
				if p.ElementId == int32(i+1) {
					parent := map[int32]string{100: "api-service", 200: "api-service-2", 300: "root"}[p.ViewId]
					want = append(want, workspace.ViewPlacement{ParentRef: parent, PositionX: p.PositionX, PositionY: p.PositionY, PositionXSet: true, PositionYSet: true})
				}
			}
			if !reflect.DeepEqual(element.Placements, want) {
				t.Fatalf("placements for %s = %#v, want %#v", ref, element.Placements, want)
			}
		}
		for ref, id := range map[string]workspace.ResourceID{"api-service": 100, "api-service-2": 200} {
			if !ws.Elements[ref].HasView || ws.Meta.Views[ref].ID != id {
				t.Fatalf("view identity lost for %s", ref)
			}
		}
		for i, source := range []string{"api-service", "api-service-2"} {
			want := &workspace.Connector{View: "api-service", Source: source, Target: "api-service-3", Label: "calls"}
			ref := workspace.ConnectorKey(want)
			connector := ws.Connectors[ref]
			if connector == nil || connector.View != want.View || connector.Source != source || connector.Target != want.Target || connector.Label != want.Label || ws.Meta.Connectors[ref].ID != workspace.ResourceID(50+i) {
				t.Fatalf("connector identity lost for %s", ref)
			}
		}
	}
	assertIdentities(got)
	if err := workspace.Save(got); err != nil {
		t.Fatal(err)
	}
	loaded, err := workspace.Load(base.Dir)
	if err != nil {
		t.Fatal(err)
	}
	assertIdentities(loaded)
	slices.Reverse(msg.Elements)
	slices.Reverse(msg.Views)
	// Placements and connector identities survive a subsequent pull as well.
	assertIdentities(ConvertExportResponse(loaded, msg))
}

func TestConvertExportResponseIgnoresCachedElementsAbsentFromExport(t *testing.T) {
	base := &workspace.Workspace{Meta: &workspace.Meta{Elements: map[string]*workspace.ResourceMetadata{"api": {ID: 99}}}}
	msg := &diagv1.ExportOrganizationResponse{
		Elements:   []*diagv1.Element{{Id: 1, Name: "API"}},
		Views:      []*diagv1.View{{Id: 100, Name: "Missing owner", OwnerElementId: new(int32(99))}},
		Placements: []*diagv1.ElementPlacement{{ElementId: 99, ViewId: 100}},
		Connectors: []*diagv1.Connector{{Id: 50, ViewId: 100, SourceElementId: 99, TargetElementId: 1}},
	}
	got := ConvertExportResponse(base, msg)
	if len(got.Elements) != 1 || got.Elements["api-1"] == nil || got.Meta.Elements["api-1"].ID != 1 || len(got.Elements["api-1"].Placements) != 0 || len(got.Connectors) != 0 || len(got.Meta.Views) != 0 {
		t.Fatalf("cached element absent from export was not reserved and ignored: %#v", got)
	}
}
