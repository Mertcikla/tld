package watch

import (
	"context"
	"fmt"
	"strings"

	tldgit "github.com/mertcikla/tld/v2/internal/git"
)

type VersionRecordRequest struct {
	RepositoryID       int64
	Status             GitStatus
	RepresentationHash string
	BaselineOnly       bool
	Logger             EventLogger
}

type VersionRecordResult struct {
	Version Version
	Created bool
}

type VersionRecorder struct {
	Store *Store
}

func NewVersionRecorder(store *Store) *VersionRecorder {
	return &VersionRecorder{Store: store}
}

func (v *VersionRecorder) RecordHead(ctx context.Context, req VersionRecordRequest) (VersionRecordResult, error) {
	if v == nil || v.Store == nil {
		return VersionRecordResult{}, fmt.Errorf("watch version recorder requires a store")
	}
	var pruneDeleted bool
	if gitStatusClean(req.Status) {
		pruneDeleted = true
	}
	latest, found, err := v.Store.LatestWatchVersion(ctx, req.RepositoryID)
	if err != nil {
		return VersionRecordResult{}, err
	}
	if found && latest.CommitHash == req.Status.HeadCommit && latest.RepresentationHash == req.RepresentationHash {
		return VersionRecordResult{Version: latest}, nil
	}
	views, elements, connectors, err := v.Store.WorkspaceResourceCounts(ctx)
	if err != nil {
		return VersionRecordResult{}, err
	}
	description := strings.TrimSpace(req.Status.HeadMessage)
	if description == "" {
		description = "tld watch " + shortHash(req.Status.HeadCommit)
	}
	workspaceVersionID, err := v.Store.CreateWorkspaceVersion(ctx, req.Status.HeadCommit, "watch", nil, views, elements, connectors, &description, &req.RepresentationHash)
	if err != nil && !strings.Contains(err.Error(), "constraint failed") {
		return VersionRecordResult{}, err
	}
	if err != nil {
		logInfo(ctx, req.Logger, "watch.workspace_version.constraint_skipped", "repository_id", req.RepositoryID, "head", req.Status.HeadCommit)
	}
	var workspaceID *int64
	if err == nil {
		workspaceID = &workspaceVersionID
	}
	parent := ""
	if repo, err := v.Store.Repository(ctx, req.RepositoryID); err == nil {
		parent, _ = tldgit.DetectParentCommit(repo.RepoRoot)
	}
	if parent == "" && found {
		parent = latest.CommitHash
	}
	var diffs []RepresentationDiff
	if !req.BaselineOnly {
		diffs, err = v.Store.BuildWatchDiffs(ctx, req.RepositoryID, req.RepresentationHash)
		if err != nil {
			return VersionRecordResult{}, err
		}
	}
	version, err := v.Store.CreateWatchVersion(ctx, req.RepositoryID, req.Status.HeadCommit, strings.TrimSpace(req.Status.HeadMessage), parent, req.Status.Branch, req.RepresentationHash, workspaceID, diffs)
	if err != nil {
		return VersionRecordResult{}, err
	}
	if pruneDeleted {
		if err := v.Store.PruneDeletedMaterializedResources(ctx, req.RepositoryID); err != nil {
			return VersionRecordResult{}, err
		}
	}
	return VersionRecordResult{Version: version, Created: true}, nil
}

func (r *Runner) versionRecorder() *VersionRecorder {
	if r == nil {
		return &VersionRecorder{}
	}
	return NewVersionRecorder(r.Store)
}

func (r *Runner) createVersionForHead(ctx context.Context, repositoryID int64, status GitStatus, representationHash string, baselineOnly bool, logger EventLogger) error {
	_, err := r.versionRecorder().RecordHead(ctx, VersionRecordRequest{
		RepositoryID:       repositoryID,
		Status:             status,
		RepresentationHash: representationHash,
		BaselineOnly:       baselineOnly,
		Logger:             logger,
	})
	return err
}
