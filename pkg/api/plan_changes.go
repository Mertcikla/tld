package api

import (
	"slices"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"google.golang.org/protobuf/proto"
)

// PlanElementChangesMetadata compares the effective patch, rather than the
// serialized messages: omitted optional fields preserve their target values.
// Owned views and placements are compared separately.
func PlanElementChangesMetadata(planned *diagv1.PlanElement, current *diagv1.Element) bool {
	if current == nil {
		return true
	}
	return (strings.TrimSpace(planned.GetName()) != "" && strings.TrimSpace(planned.GetName()) != current.GetName()) ||
		patchStringChanged(planned.Kind, current.GetKind()) ||
		patchStringChanged(planned.Description, current.GetDescription()) ||
		patchStringChanged(planned.Technology, current.GetTechnology()) ||
		patchStringChanged(planned.Url, current.GetUrl()) ||
		patchStringChanged(planned.LogoUrl, current.GetLogoUrl()) ||
		patchStringChanged(planned.Repo, current.GetRepo()) ||
		patchStringChanged(planned.RepositoryId, current.GetRepositoryId()) ||
		patchStringChanged(planned.Branch, current.GetBranch()) ||
		patchStringChanged(planned.Language, current.GetLanguage()) ||
		patchStringChanged(planned.FilePath, current.GetFilePath()) ||
		(planned.BypassNoiseGate != nil && planned.GetBypassNoiseGate() != current.GetBypassNoiseGate()) ||
		(planned.Tags != nil && !slices.Equal(planned.Tags, current.GetTags())) ||
		(planned.TechnologyLinks != nil && !slices.EqualFunc(planned.TechnologyLinks, current.GetTechnologyLinks(), func(a, b *diagv1.TechnologyLink) bool { return proto.Equal(a, b) }))
}

// PlanConnectorChangesMetadata compares a connector patch after resolving refs.
// Visibility overrides are separate from connector metadata.
func PlanConnectorChangesMetadata(planned *diagv1.PlanConnector, current *diagv1.Connector, viewID, sourceID, targetID int32) bool {
	if current == nil {
		return true
	}
	return viewID != current.GetViewId() || sourceID != current.GetSourceElementId() || targetID != current.GetTargetElementId() ||
		patchStringChanged(planned.Label, current.GetLabel()) ||
		patchStringChanged(planned.Description, current.GetDescription()) ||
		patchStringChanged(planned.Relationship, current.GetRelationship()) ||
		patchStringChanged(planned.Direction, current.GetDirection()) ||
		patchStringChanged(planned.Style, current.GetStyle()) ||
		patchStringChanged(planned.Url, current.GetUrl()) ||
		patchStringChanged(planned.SourceHandle, current.GetSourceHandle()) ||
		patchStringChanged(planned.TargetHandle, current.GetTargetHandle())
}

func patchStringChanged(patch *string, current string) bool {
	return patch != nil && *patch != current
}
