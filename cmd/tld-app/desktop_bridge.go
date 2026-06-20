package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type DesktopBridge struct {
	ctx context.Context
}

type DialogFilter struct {
	DisplayName string `json:"displayName"`
	Pattern     string `json:"pattern"`
}

type FileDialogResult struct {
	Path     string `json:"path"`
	Content  string `json:"content"`
	Canceled bool   `json:"canceled"`
}

type SaveFileResult struct {
	Path     string `json:"path"`
	Canceled bool   `json:"canceled"`
}

func NewDesktopBridge() *DesktopBridge {
	return &DesktopBridge{}
}

func (b *DesktopBridge) startup(ctx context.Context) {
	b.ctx = ctx
}

func (b *DesktopBridge) SaveFile(defaultFilename string, filters []DialogFilter, base64Content string) (SaveFileResult, error) {
	if b.ctx == nil {
		return SaveFileResult{}, errors.New("desktop bridge is not ready")
	}
	content, err := decodeBase64Content(base64Content)
	if err != nil {
		return SaveFileResult{}, err
	}
	path, err := wailsruntime.SaveFileDialog(b.ctx, wailsruntime.SaveDialogOptions{
		Title:                "Save File",
		DefaultFilename:      sanitizeDefaultFilename(defaultFilename),
		CanCreateDirectories: true,
		Filters:              toWailsFilters(filters),
	})
	if err != nil {
		return SaveFileResult{}, err
	}
	if strings.TrimSpace(path) == "" {
		return SaveFileResult{Canceled: true}, nil
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return SaveFileResult{}, err
	}
	return SaveFileResult{Path: path}, nil
}

func (b *DesktopBridge) OpenTextFile(filters []DialogFilter) (FileDialogResult, error) {
	if b.ctx == nil {
		return FileDialogResult{}, errors.New("desktop bridge is not ready")
	}
	path, err := wailsruntime.OpenFileDialog(b.ctx, wailsruntime.OpenDialogOptions{
		Title:           "Open File",
		Filters:         toWailsFilters(filters),
		ResolvesAliases: true,
	})
	if err != nil {
		return FileDialogResult{}, err
	}
	if strings.TrimSpace(path) == "" {
		return FileDialogResult{Canceled: true}, nil
	}
	return readTextFile(path)
}

func (b *DesktopBridge) PickWritableMarkdownFile() (FileDialogResult, error) {
	if b.ctx == nil {
		return FileDialogResult{}, errors.New("desktop bridge is not ready")
	}
	path, err := wailsruntime.OpenFileDialog(b.ctx, wailsruntime.OpenDialogOptions{
		Title: "Choose Markdown File",
		Filters: toWailsFilters([]DialogFilter{
			{DisplayName: "Markdown Files (*.md;*.markdown;*.mdx)", Pattern: "*.md;*.markdown;*.mdx"},
		}),
		ResolvesAliases: true,
	})
	if err != nil {
		return FileDialogResult{}, err
	}
	if strings.TrimSpace(path) == "" {
		return FileDialogResult{Canceled: true}, nil
	}
	return writableMarkdownFile(path)
}

func (b *DesktopBridge) ReadTextFile(path string) (FileDialogResult, error) {
	return readTextFile(path)
}

func (b *DesktopBridge) OpenPath(path string) error {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return errors.New("path is required")
	}
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", cleanPath).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", cleanPath).Start()
	default:
		return exec.Command("xdg-open", cleanPath).Start()
	}
}

func decodeBase64Content(value string) ([]byte, error) {
	content, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode file content: %w", err)
	}
	return content, nil
}

func readTextFile(path string) (FileDialogResult, error) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return FileDialogResult{}, errors.New("path is required")
	}
	info, err := os.Stat(cleanPath)
	if err != nil {
		return FileDialogResult{}, err
	}
	if info.IsDir() {
		return FileDialogResult{}, errors.New("path must point to a file")
	}
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return FileDialogResult{}, err
	}
	return FileDialogResult{Path: cleanPath, Content: string(content)}, nil
}

func writableMarkdownFile(path string) (FileDialogResult, error) {
	cleanPath := strings.TrimSpace(path)
	if cleanPath == "" {
		return FileDialogResult{}, errors.New("path is required")
	}
	if !isMarkdownFilePath(cleanPath) {
		return FileDialogResult{}, errors.New("path must point to a markdown file")
	}
	info, err := os.Stat(cleanPath)
	if err != nil {
		return FileDialogResult{}, err
	}
	if !info.Mode().IsRegular() {
		return FileDialogResult{}, errors.New("path must point to a regular file")
	}
	if info.Mode().Perm()&0o222 == 0 {
		return FileDialogResult{}, errors.New("markdown file is not writable")
	}
	file, err := os.OpenFile(cleanPath, os.O_WRONLY, 0)
	if err != nil {
		if os.IsPermission(err) {
			return FileDialogResult{}, fmt.Errorf("markdown file is not writable: %w", err)
		}
		return FileDialogResult{}, err
	}
	if err := file.Close(); err != nil {
		return FileDialogResult{}, err
	}
	return FileDialogResult{Path: cleanPath}, nil
}

func isMarkdownFilePath(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	return strings.HasSuffix(lower, ".md") ||
		strings.HasSuffix(lower, ".markdown") ||
		strings.HasSuffix(lower, ".mdx")
}

func sanitizeDefaultFilename(value string) string {
	name := filepath.Base(strings.TrimSpace(value))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "untitled"
	}
	return name
}

func toWailsFilters(filters []DialogFilter) []wailsruntime.FileFilter {
	if len(filters) == 0 {
		return nil
	}
	out := make([]wailsruntime.FileFilter, 0, len(filters))
	for _, filter := range filters {
		name := strings.TrimSpace(filter.DisplayName)
		pattern := strings.TrimSpace(filter.Pattern)
		if name == "" || pattern == "" {
			continue
		}
		out = append(out, wailsruntime.FileFilter{DisplayName: name, Pattern: pattern})
	}
	return out
}
