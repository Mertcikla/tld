package selfupdate

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestShellInstallerPreservesExistingBinaryOnFailedDownload(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix installer")
	}
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "download-failure"}[fail], func(t *testing.T) {
			dir := t.TempDir()
			install := filepath.Join(dir, "install with spaces")
			mock := filepath.Join(dir, "mock")
			for _, path := range []string{install, mock} {
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			destination := filepath.Join(install, "tld")
			if err := os.WriteFile(destination, []byte("old"), 0o755); err != nil {
				t.Fatal(err)
			}
			archive := filepath.Join(dir, "release.tar.gz")
			file, err := os.Create(archive)
			if err != nil {
				t.Fatal(err)
			}
			gz := gzip.NewWriter(file)
			tw := tar.NewWriter(gz)
			body := "#!/bin/sh\necho new-version\n"
			if err := tw.WriteHeader(&tar.Header{Name: "tld", Mode: 0o755, Size: int64(len(body))}); err != nil {
				t.Fatal(err)
			}
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			if err := gz.Close(); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			curl := "#!/bin/sh\nfor arg do destination=\"$arg\"; done\nworkdir=$(dirname \"$destination\")\nmkdir \"$workdir/unexpected\"\nprintf keep > \"$workdir/unexpected/sentinel\"\ncp \"$TEST_ARCHIVE\" \"$destination\"\n"
			if fail {
				curl += "exit 22\n"
			}
			if err := os.WriteFile(filepath.Join(mock, "curl"), []byte(curl), 0o755); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", "../../scripts/install.sh")
			cmd.Env = append(os.Environ(), "PATH="+mock+":"+os.Getenv("PATH"), "INSTALL_DIR="+install, "TEST_ARCHIVE="+archive, "TMPDIR="+dir)
			out, err := cmd.CombinedOutput()
			if (err != nil) != fail {
				t.Fatalf("installer: %v\n%s", err, out)
			}
			workdirs, err := filepath.Glob(filepath.Join(dir, "tld-install.*"))
			if err != nil || len(workdirs) != 1 {
				t.Fatalf("expected cleanup to preserve unexpected contents: %v, %v", workdirs, err)
			}
			sentinel, err := os.ReadFile(filepath.Join(workdirs[0], "unexpected", "sentinel"))
			if err != nil || string(sentinel) != "keep" {
				t.Fatalf("cleanup removed unrelated data: %q, %v", sentinel, err)
			}
			entries, err := os.ReadDir(workdirs[0])
			if err != nil || len(entries) != 1 || entries[0].Name() != "unexpected" {
				t.Fatalf("cleanup did not remove installer-owned files: %v, %v", entries, err)
			}
			got, err := os.ReadFile(destination)
			want := body
			if fail {
				want = "old"
			}
			if err != nil || string(got) != want {
				t.Fatalf("installed contents = %q, %v", got, err)
			}
		})
	}
}
