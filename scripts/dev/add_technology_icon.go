package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type catalogItem struct {
	IconURL     string `json:"iconUrl,omitempty"`
	Name        string `json:"name"`
	Provider    string `json:"provider,omitempty"`
	DocsURL     string `json:"docsUrl,omitempty"`
	Description string `json:"description,omitempty"`
	WebsiteURL  string `json:"websiteUrl,omitempty"`
	NameShort   string `json:"nameShort"`
	DefaultSlug string `json:"defaultSlug"`
}

type validationCatalogItem struct {
	Name        string `json:"name"`
	NameShort   string `json:"nameShort"`
	DefaultSlug string `json:"defaultSlug"`
}

type archiveEntry struct {
	name     string
	body     []byte
	mode     int64
	typeflag byte
}

var slugPartRE = regexp.MustCompile(`[^a-z0-9]+`)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "add technology icon: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		name        = flag.String("name", "", "technology name")
		nameShort   = flag.String("short", "", "short display name; defaults to -name")
		slug        = flag.String("slug", "", "icon/catalog slug; defaults to a slugified -name")
		iconPath    = flag.String("icon", "", "path to a PNG icon")
		provider    = flag.String("provider", "", "optional provider, for example aws, azure, gcp")
		docsURL     = flag.String("docs-url", "", "optional documentation URL")
		websiteURL  = flag.String("website-url", "", "optional website URL")
		description = flag.String("description", "", "optional catalog description")
		archivePath = flag.String("archive", filepath.Join("build-assets", "icons.tar.gz"), "path to icons tar.gz")
		techCatalog = flag.String("tech-catalog", filepath.Join("internal", "tech", "icons.json"), "path to backend validation catalog")
		replace     = flag.Bool("replace", false, "replace an existing catalog/icon entry with the same slug")
	)
	flag.Parse()

	itemName := strings.TrimSpace(*name)
	if itemName == "" {
		return errors.New("-name is required")
	}
	if strings.TrimSpace(*iconPath) == "" {
		return errors.New("-icon is required")
	}

	itemSlug := strings.TrimSpace(*slug)
	if itemSlug == "" {
		itemSlug = slugify(itemName)
	} else {
		itemSlug = slugify(itemSlug)
	}
	if itemSlug == "" {
		return fmt.Errorf("could not derive a slug from %q", itemName)
	}

	iconBody, err := os.ReadFile(*iconPath)
	if err != nil {
		return err
	}
	if !isPNG(iconBody) {
		return fmt.Errorf("%s is not a PNG; catalog icons must be PNG files", *iconPath)
	}

	entries, err := readArchive(*archivePath)
	if err != nil {
		return err
	}

	catalog, err := readCatalog(entries)
	if err != nil {
		return err
	}

	short := strings.TrimSpace(*nameShort)
	if short == "" {
		short = itemName
	}
	item := catalogItem{
		IconURL:     "/icons/" + itemSlug + ".png",
		Name:        itemName,
		Provider:    strings.TrimSpace(*provider),
		DocsURL:     strings.TrimSpace(*docsURL),
		Description: strings.TrimSpace(*description),
		WebsiteURL:  strings.TrimSpace(*websiteURL),
		NameShort:   short,
		DefaultSlug: itemSlug,
	}

	updatedCatalog, err := upsertCatalogItem(catalog, item, *replace)
	if err != nil {
		return err
	}

	catalogJSON, err := marshalJSON(updatedCatalog)
	if err != nil {
		return err
	}

	iconName := "icons/" + itemSlug + ".png"
	updatedEntries := upsertArchiveEntry(entries, archiveEntry{name: iconName, body: iconBody, mode: 0o644, typeflag: tar.TypeReg}, *replace)
	updatedEntries = upsertArchiveEntry(updatedEntries, archiveEntry{name: "icons.json", body: catalogJSON, mode: 0o644, typeflag: tar.TypeReg}, true)

	if err := writeArchive(*archivePath, updatedEntries); err != nil {
		return err
	}
	if err := writeValidationCatalog(*techCatalog, updatedCatalog); err != nil {
		return err
	}

	fmt.Printf("Added %s (%s) to %s\n", item.Name, item.DefaultSlug, *archivePath)
	return nil
}

func slugify(value string) string {
	slug := strings.ToLower(strings.TrimSpace(value))
	slug = slugPartRE.ReplaceAllString(slug, "-")
	return strings.Trim(slug, "-")
}

func isPNG(body []byte) bool {
	return len(body) >= 8 &&
		body[0] == 0x89 &&
		body[1] == 'P' &&
		body[2] == 'N' &&
		body[3] == 'G' &&
		body[4] == '\r' &&
		body[5] == '\n' &&
		body[6] == 0x1a &&
		body[7] == '\n'
}

func readArchive(path string) ([]archiveEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer func() { _ = gzr.Close() }()

	var entries []archiveEntry
	tr := tar.NewReader(gzr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return entries, nil
		}
		if err != nil {
			return nil, err
		}

		entry := archiveEntry{name: filepath.ToSlash(filepath.Clean(hdr.Name)), mode: hdr.Mode, typeflag: hdr.Typeflag}
		if hdr.Typeflag == tar.TypeReg {
			entry.body, err = io.ReadAll(tr)
			if err != nil {
				return nil, err
			}
		}
		entries = append(entries, entry)
	}
}

func readCatalog(entries []archiveEntry) ([]catalogItem, error) {
	for _, entry := range entries {
		if entry.name != "icons.json" {
			continue
		}
		var items []catalogItem
		if err := json.Unmarshal(entry.body, &items); err != nil {
			return nil, err
		}
		return items, nil
	}
	return nil, errors.New("icons.json not found in archive")
}

func upsertCatalogItem(items []catalogItem, item catalogItem, replace bool) ([]catalogItem, error) {
	out := make([]catalogItem, 0, len(items)+1)
	replaced := false
	for _, existing := range items {
		if existing.DefaultSlug != item.DefaultSlug {
			out = append(out, existing)
			continue
		}
		if !replace {
			return nil, fmt.Errorf("catalog entry %q already exists; rerun with -replace to overwrite it", item.DefaultSlug)
		}
		out = append(out, item)
		replaced = true
	}
	if !replaced {
		out = append(out, item)
	}
	return out, nil
}

func upsertArchiveEntry(entries []archiveEntry, item archiveEntry, replace bool) []archiveEntry {
	out := make([]archiveEntry, 0, len(entries)+1)
	replaced := false
	for _, entry := range entries {
		if entry.name != item.name {
			out = append(out, entry)
			continue
		}
		if replace {
			out = append(out, item)
		} else {
			out = append(out, entry)
		}
		replaced = true
	}
	if !replaced {
		out = append(out, item)
	}
	return out
}

func writeArchive(path string, entries []archiveEntry) error {
	var buf bytes.Buffer
	gzw, err := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if err != nil {
		return err
	}
	gzw.Name = filepath.Base(path)
	gzw.ModTime = time.Unix(0, 0)

	tw := tar.NewWriter(gzw)
	for _, entry := range entries {
		mode := entry.mode
		if mode == 0 {
			mode = 0o644
		}
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		hdr := &tar.Header{
			Name:     entry.name,
			Mode:     mode,
			Typeflag: typeflag,
			ModTime:  time.Unix(0, 0),
		}
		if typeflag == tar.TypeReg {
			hdr.Size = int64(len(entry.body))
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if typeflag == tar.TypeReg {
			if _, err := tw.Write(entry.body); err != nil {
				return err
			}
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gzw.Close(); err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeValidationCatalog(path string, items []catalogItem) error {
	validationItems := make([]validationCatalogItem, 0, len(items))
	for _, item := range items {
		validationItems = append(validationItems, validationCatalogItem{
			Name:        item.Name,
			NameShort:   item.NameShort,
			DefaultSlug: item.DefaultSlug,
		})
	}
	body, err := marshalJSON(validationItems)
	if err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func marshalJSON(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
