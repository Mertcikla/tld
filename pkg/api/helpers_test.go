package api

import (
	"context"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
)

func TestResolveWorkspaceIDPrefersMessageIDAndRejectsEmptyContext(t *testing.T) {
	ctxID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	msgID := uuid.MustParse("22222222-2222-2222-2222-222222222222")

	got, err := ResolveWorkspaceID(WithWorkspaceID(context.Background(), ctxID), " "+msgID.String()+" ")
	if err != nil {
		t.Fatalf("ResolveWorkspaceID returned error: %v", err)
	}
	if got != msgID {
		t.Fatalf("message org_id should win over context: got %s want %s", got, msgID)
	}

	_, err = ResolveWorkspaceID(context.Background(), "")
	if connect.CodeOf(err) != connect.CodeInvalidArgument || !strings.Contains(err.Error(), "org_id") {
		t.Fatalf("expected invalid org_id error, got %v", err)
	}
}

func TestConvertTechnologyLinksNormalizesDefaultsAndDefendsUniqueness(t *testing.T) {
	catalogSlug := " Go "
	customSlug := "ignored"
	links := []*diagv1.TechnologyLink{
		{Slug: &catalogSlug, Label: " Go ", IsPrimaryIcon: true},
		{Type: " CUSTOM ", Slug: &customSlug, Label: " External API "},
	}

	got, err := ConvertTechnologyLinks(links)
	if err != nil {
		t.Fatalf("ConvertTechnologyLinks returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 links, got %#v", got)
	}
	if got[0].GetType() != "catalog" || got[0].GetSlug() != "Go" || got[0].GetLabel() != "Go" || !got[0].GetIsPrimaryIcon() {
		t.Fatalf("catalog link was not normalized: %#v", got[0])
	}
	if got[1].GetType() != "custom" || got[1].Slug != nil || got[1].GetLabel() != "External API" {
		t.Fatalf("custom link was not normalized: %#v", got[1])
	}

	dupSlug := "go"
	_, err = ConvertTechnologyLinks([]*diagv1.TechnologyLink{
		{Type: "catalog", Slug: &catalogSlug, Label: "Go"},
		{Type: "catalog", Slug: &dupSlug, Label: "Golang"},
	})
	if connect.CodeOf(err) != connect.CodeInvalidArgument || !strings.Contains(err.Error(), "duplicate catalog technology slug") {
		t.Fatalf("expected duplicate catalog validation error, got %v", err)
	}

	_, err = ConvertTechnologyLinks([]*diagv1.TechnologyLink{{Type: "custom", Label: "Team SDK", IsPrimaryIcon: true}})
	if connect.CodeOf(err) != connect.CodeInvalidArgument || !strings.Contains(err.Error(), "custom technology cannot be the primary icon") {
		t.Fatalf("expected custom primary validation error, got %v", err)
	}
}
