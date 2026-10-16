package server

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
)

// BrowseRepositoryFolders lists directories on the same machine as the indexer.
func (s *repositoryService) BrowseRepositoryFolders(ctx context.Context, req *connect.Request[pb.BrowseRepositoryFoldersRequest]) (*connect.Response[pb.BrowseRepositoryFoldersResponse], error) {
	if s.selfHosted {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("folder browsing is only available on local servers"))
	}
	path := strings.TrimSpace(req.Msg.GetPath())
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		path = home
	}
	root, err := resolveRepositoryRoot(path)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("cannot browse folder: %w", err))
	}
	result := &pb.BrowseRepositoryFoldersResponse{Path: root, ParentPath: filepath.Dir(root)}
	if result.ParentPath == root {
		result.ParentPath = ""
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		isDir := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(filepath.Join(root, entry.Name()))
			isDir = err == nil && info.IsDir()
		}
		if isDir {
			result.Folders = append(result.Folders, entry.Name())
		}
	}
	return connect.NewResponse(result), nil
}

func repositoryAddWarnings(ctx context.Context, root string, requirements []*pb.IndexerRequirement) []string {
	var warnings []string
	// A parent checkout must not make a selected subdirectory look like a Git root.
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		warnings = append(warnings, "No Git repository was found at this folder's root. You can still add it, but Git history may be unavailable.")
	} else if _, err := repositoryGit(ctx, root, "rev-parse", "--git-dir"); err != nil {
		warnings = append(warnings, "The Git repository at this folder's root could not be read. You can still add it, but Git history may be unavailable.")
	}
	if len(requirements) == 0 {
		warnings = append(warnings, "No required indexers were detected. Check that you selected the project folder; you can still add it.")
	}
	return warnings
}
