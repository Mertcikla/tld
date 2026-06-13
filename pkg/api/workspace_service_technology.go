package api

import (
	"context"
	"fmt"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/tech"
)

func (s *WorkspaceService) CreateCustomTechnology(
	ctx context.Context,
	req *connect.Request[diagv1.CreateCustomTechnologyRequest],
) (*connect.Response[diagv1.CreateCustomTechnologyResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckWrite(ctx, workspaceID, "elements"); err != nil {
		return nil, err
	}

	m := req.Msg
	item, err := tech.CreateCustomTechnology(tech.CustomTechnologyInput{
		Name:          m.GetName(),
		NameShort:     m.GetNameShort(),
		Aliases:       m.GetAliases(),
		Icon:          m.GetIcon(),
		MediaType:     m.GetMediaType(),
		PreferredSlug: m.GetPreferredSlug(),
	})
	if err != nil {
		if tech.IsInvalidCustomTechnology(err) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("create custom technology: %w", err))
	}

	protoItem := technologyCatalogItemToProto(item)
	resp := &diagv1.CreateCustomTechnologyResponse{Item: protoItem}
	s.hooks().AfterWrite(ctx, workspaceID, "create", "technology", protoItem.GetDefaultSlug(), map[string]any{"name": protoItem.GetName()}, resp)

	return connect.NewResponse(resp), nil
}

func technologyCatalogItemToProto(item tech.CatalogItem) *diagv1.TechnologyCatalogItem {
	out := &diagv1.TechnologyCatalogItem{
		IconUrl:     item.IconURL,
		Name:        item.Name,
		NameShort:   item.NameShort,
		DefaultSlug: item.DefaultSlug,
		Aliases:     append([]string{}, item.Aliases...),
	}
	if item.Provider != "" {
		out.Provider = &item.Provider
	}
	if item.DocsURL != "" {
		out.DocsUrl = &item.DocsURL
	}
	if item.Description != "" {
		out.Description = &item.Description
	}
	if item.WebsiteURL != "" {
		out.WebsiteUrl = &item.WebsiteURL
	}
	return out
}
