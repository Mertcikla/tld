package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	"github.com/mertcikla/tld/v2/internal/codeindex/mapconfig"
	"github.com/mertcikla/tld/v2/internal/repolink"
)

func (s *codeIndexRepositoryService) GetRepositorySettings(ctx context.Context, req *connect.Request[pb.GetRepositorySettingsRequest]) (*connect.Response[pb.RepositorySettings], error) {
	settings, err := s.repositorySettings(ctx, req.Msg.RepositoryId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(settings), nil
}

func (s *codeIndexRepositoryService) repositorySettings(ctx context.Context, repositoryID string) (*pb.RepositorySettings, error) {
	repo, err := s.store.Repository(ctx, repositoryID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	overrides, err := s.store.RepositoryMapOverrides(ctx, repositoryID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defaults := mapconfig.FromGlobal(s.config)
	effective, err := defaults.WithOverrides(overrides)
	settings := &pb.RepositorySettings{MapDefaults: defaults.Configuration(), MapOverrides: overrides, EffectiveMap: effective.Configuration()}
	if err != nil {
		settings.MapValidationError = err.Error()
	}
	if _, err := repositoryGit(ctx, repo.Root, "rev-parse", "--git-dir"); err != nil {
		return settings, nil
	}
	settings.IsGit = true
	branch, _ := repositoryGit(ctx, repo.Root, "symbolic-ref", "--quiet", "--short", "HEAD")
	settings.CurrentBranch = strings.TrimSpace(branch)
	head, _ := repositoryGit(ctx, repo.Root, "rev-parse", "--verify", "HEAD")
	settings.HeadSha = strings.TrimSpace(head)
	remotes, err := repositoryGit(ctx, repo.Root, "remote")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	for _, name := range strings.Fields(remotes) {
		fetch, err := remoteConfigURLs(ctx, repo.Root, name, "url")
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		push, err := remoteConfigURLs(ctx, repo.Root, name, "pushurl")
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		settings.Remotes = append(settings.Remotes, &pb.RepositoryRemote{Name: name, FetchUrls: fetch, PushUrls: push})
	}
	return settings, nil
}

func (s *codeIndexRepositoryService) UpdateRepositoryMapConfiguration(ctx context.Context, req *connect.Request[pb.UpdateRepositoryMapConfigurationRequest]) (*connect.Response[pb.RepositorySettings], error) {
	id := req.Msg.RepositoryId
	if _, err := s.store.Repository(ctx, id); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if _, err := mapconfig.FromGlobal(s.config).WithOverrides(req.Msg.Overrides); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	ctx, release, err := s.store.AcquireLease(ctx, id)
	if err != nil {
		return nil, impactError(err)
	}
	defer release()
	if err := s.store.SaveRepositoryMapOverrides(ctx, id, req.Msg.Overrides); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return s.GetRepositorySettings(ctx, connect.NewRequest(&pb.GetRepositorySettingsRequest{RepositoryId: id}))
}

var remoteNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

func validateRemote(remote *pb.RepositoryRemote, remove bool) error {
	if remote == nil || !remoteNamePattern.MatchString(remote.Name) || strings.Contains(remote.Name, "..") || strings.Contains(remote.Name, "//") || strings.HasSuffix(remote.Name, "/") || strings.HasSuffix(remote.Name, ".lock") {
		return fmt.Errorf("a valid remote name is required")
	}
	if remove {
		return nil
	}
	if len(remote.FetchUrls) == 0 {
		return fmt.Errorf("at least one fetch URL is required")
	}
	for _, urls := range [][]string{remote.FetchUrls, remote.PushUrls} {
		for _, url := range urls {
			if strings.TrimSpace(url) == "" || strings.ContainsAny(url, "\x00\r\n") {
				return fmt.Errorf("remote URLs must be nonempty single-line values")
			}
		}
	}
	return nil
}

func remoteConfigURLs(ctx context.Context, root, name, key string) ([]string, error) {
	raw, err := repositoryGit(ctx, root, "config", "--local", "--get-all", "remote."+name+"."+key)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.TrimSuffix(raw, "\n"), "\n"), nil
}

func (s *codeIndexRepositoryService) UpdateRepositoryRemote(ctx context.Context, req *connect.Request[pb.UpdateRepositoryRemoteRequest]) (*connect.Response[pb.RepositorySettings], error) {
	if err := validateRemote(req.Msg.Remote, req.Msg.Remove); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	repo, err := s.store.Repository(ctx, req.Msg.RepositoryId)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	ctx, release, err := s.store.AcquireLease(ctx, repo.Id)
	if err != nil {
		return nil, impactError(err)
	}
	defer release()
	if _, err := repositoryGit(ctx, repo.Root, "rev-parse", "--git-dir"); err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("repository is not an available Git checkout"))
	}
	remote := req.Msg.Remote
	if _, err := repositoryGit(ctx, repo.Root, "check-ref-format", "refs/remotes/"+remote.Name+"/HEAD"); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid remote name"))
	}
	if req.Msg.Remove {
		_, err = repositoryGit(ctx, repo.Root, "remote", "remove", remote.Name)
	} else {
		err = replaceRemoteURLs(ctx, repo.Root, remote)
	}
	if err != nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	if remoteURL := repolink.GitRemoteURL(ctx, repo.Root); remoteURL != "" {
		_ = s.store.SetRepositoryRemoteURL(ctx, repo.Id, remoteURL)
	}
	return s.GetRepositorySettings(ctx, connect.NewRequest(&pb.GetRepositorySettingsRequest{RepositoryId: repo.Id}))
}

func replaceRemoteURLs(ctx context.Context, root string, remote *pb.RepositoryRemote) error {
	// Hold Git's config lock and stage all URL edits before replacing config.
	// A failed multi-URL edit must not leave a partially changed remote behind.
	rawPath, err := repositoryGit(ctx, root, "rev-parse", "--git-path", "config")
	if err != nil {
		return err
	}
	configPath := strings.TrimSpace(rawPath)
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(root, configPath)
	}
	lock, err := os.OpenFile(configPath+".lock", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("could not lock Git config: %w", err)
	}
	defer func() { _ = lock.Close(); _ = os.Remove(configPath + ".lock") }()
	config, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(configPath)
	if err != nil {
		return err
	}
	staged, err := os.CreateTemp(filepath.Dir(configPath), "tld-remote-config-*")
	if err != nil {
		return err
	}
	defer func() { _ = staged.Close(); _ = os.Remove(staged.Name()) }()
	if err := staged.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if _, err := staged.Write(config); err != nil {
		return err
	}
	if err := staged.Close(); err != nil {
		return err
	}
	configure := func(args ...string) (string, error) {
		return repositoryGit(ctx, root, append([]string{"config", "--file", staged.Name()}, args...)...)
	}
	_, err = configure("--get-regexp", `^remote\.`+regexp.QuoteMeta(remote.Name)+`\.`)
	var exit *exec.ExitError
	if err != nil {
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return err
		}
		if _, err := configure("--add", "remote."+remote.Name+".fetch", "+refs/heads/*:refs/remotes/"+remote.Name+"/*"); err != nil {
			return err
		}
	}
	for _, field := range []struct {
		key  string
		urls []string
	}{{"url", remote.FetchUrls}, {"pushurl", remote.PushUrls}} {
		key := "remote." + remote.Name + "." + field.key
		if len(field.urls) == 0 {
			_, err := configure("--unset-all", key)
			var exit *exec.ExitError
			if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 5) {
				return err
			}
			continue
		}
		if _, err := configure("--replace-all", key, field.urls[0]); err != nil {
			return err
		}
		for _, url := range field.urls[1:] {
			if _, err := configure("--add", key, url); err != nil {
				return err
			}
		}
	}
	return os.Rename(staged.Name(), configPath)
}
