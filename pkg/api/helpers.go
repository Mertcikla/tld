package api

import (
	"errors"
	"fmt"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// ResolveWorkspaceID resolves the org_id from a message field or falls back to context.
// It is used in every RPC that accepts an optional org_id field.
func ResolveWorkspaceID(ctx interface{ Value(any) any }, msgWorkspaceID string) (uuid.UUID, error) {
	if msgWorkspaceID != "" {
		return parseRequiredUUID("org_id", msgWorkspaceID)
	}
	id, _ := ctx.Value(ctxKeyWorkspaceID).(uuid.UUID)
	if id == uuid.Nil {
		return uuid.Nil, invalidArg("org_id", "must not be empty")
	}
	return id, nil
}

func parseRequiredUUID(field, value string) (uuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return uuid.Nil, invalidArgF(field, "must not be empty")
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, invalidArgF(field, "must be a valid UUID, got %q", value)
	}
	if id == uuid.Nil {
		return uuid.Nil, invalidArgF(field, "must not be the nil UUID")
	}
	return id, nil
}

func parseRequiredInt32(field string, value int32) (int32, error) {
	if value <= 0 {
		return 0, invalidArgF(field, "must be a positive integer")
	}
	return value, nil
}

var validDirections = map[string]struct{}{
	"forward":  {},
	"backward": {},
	"both":     {},
	"none":     {},
}

var validEdgeTypes = map[string]struct{}{
	"bezier":     {},
	"straight":   {},
	"step":       {},
	"smoothstep": {},
}

func validateDirection(d string) error {
	if _, ok := validDirections[d]; !ok {
		return invalidArgF("direction", "must be one of: forward, backward, both, none; got %q", d)
	}
	return nil
}

func validateEdgeType(t string) error {
	if _, ok := validEdgeTypes[t]; !ok {
		return invalidArgF("edge_type", "must be one of: bezier, straight, step, smoothstep; got %q", t)
	}
	return nil
}

// ConvertTechnologyLinks validates and converts proto TechnologyLink messages.
func ConvertTechnologyLinks(links []*diagv1.TechnologyLink) ([]*diagv1.TechnologyLink, error) {
	if len(links) == 0 {
		return []*diagv1.TechnologyLink{}, nil
	}
	if len(links) > 3 {
		return nil, invalidArg("technology_links", "max 3 technology links allowed")
	}

	result := make([]*diagv1.TechnologyLink, 0, len(links))
	seenCatalog := map[string]struct{}{}
	seenCustom := map[string]struct{}{}
	primaryCount := 0

	for i, l := range links {
		field := fmt.Sprintf("technology_links[%d]", i)
		typ := strings.ToLower(strings.TrimSpace(l.GetType()))
		slug := ""
		if l.Slug != nil {
			slug = strings.TrimSpace(l.GetSlug())
		}
		label := strings.TrimSpace(l.GetLabel())

		if typ == "" {
			if slug != "" {
				typ = "catalog"
			} else {
				typ = "custom"
			}
		}

		switch typ {
		case "catalog":
			if slug == "" {
				return nil, invalidArgF(field+".slug", "catalog technology requires a non-empty slug")
			}
			if label == "" {
				return nil, invalidArgF(field+".label", "catalog technology requires a non-empty label")
			}
			key := strings.ToLower(slug)
			if _, dup := seenCatalog[key]; dup {
				return nil, invalidArgF(field, "duplicate catalog technology slug %q", slug)
			}
			seenCatalog[key] = struct{}{}
		case "custom":
			if label == "" {
				return nil, invalidArgF(field+".label", "custom technology requires a non-empty label")
			}
			if l.GetIsPrimaryIcon() {
				return nil, invalidArgF(field, "custom technology cannot be the primary icon")
			}
			slug = ""
			key := strings.ToLower(label)
			if _, dup := seenCustom[key]; dup {
				return nil, invalidArgF(field, "duplicate custom technology label %q", label)
			}
			seenCustom[key] = struct{}{}
		default:
			return nil, invalidArgF(field+".type", "must be \"catalog\" or \"custom\", got %q", typ)
		}

		if l.GetIsPrimaryIcon() {
			primaryCount++
		}

		out := &diagv1.TechnologyLink{
			Type:          typ,
			Label:         label,
			IsPrimaryIcon: l.GetIsPrimaryIcon(),
		}
		if slug != "" {
			out.Slug = &slug
		}
		result = append(result, out)
	}

	if primaryCount > 1 {
		return nil, invalidArg("technology_links", "only one technology link may be marked as primary icon")
	}
	return result, nil
}

// OptStr returns nil when s is empty, otherwise returns a pointer to s.
func OptStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func invalidArg(field, msg string) *connect.Error {
	return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("field %q: %s", field, msg))
}

func invalidArgF(field, format string, args ...any) *connect.Error {
	return connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("field %q: %s", field, fmt.Sprintf(format, args...)))
}

func storeErr(op string, err error) error {
	if errors.Is(err, ErrUnimplemented) {
		return connect.NewError(connect.CodeUnimplemented, err)
	}
	return connect.NewError(connect.CodeInternal, fmt.Errorf("%s: %w", op, err))
}
