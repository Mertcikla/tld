//go:build cliworkflow

package cliworkflow

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLifecycleDatabaseOnlyAndConcurrentEdits(t *testing.T) {
	for _, backend := range []string{"sqlite", "remote", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			if backend == "postgres" && os.Getenv("TLD_TEST_POSTGRES_URL") == "" {
				t.Skip("set TLD_TEST_POSTGRES_URL")
			}
			h := newHarnessFor(t, backend)
			h.add("api", "root", 100, 100)
			h.add("db", "root", 400, 100)
			beforeYAML := h.yamlGraph().canonical()
			// Distinct clients share the target but have no shared YAML cache to race.
			var wg sync.WaitGroup
			type result struct {
				out string
				err error
			}
			results := make([]result, 2)
			for i, ref := range []string{"api", "db"} {
				wg.Add(1)
				go func(i int, ref string) {
					defer wg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
					defer cancel()
					c := exec.CommandContext(ctx, binary, "--workspace", filepath.Join(h.dir, "db-only"), "--target", h.targetName, "--data-dir", filepath.Join(h.dir, "data"), "--format", "json", "update", "element", strconv.Itoa(int(h.ids.Elements[ref])), "description", "concurrent "+ref)
					c.Dir = h.dir
					c.Env = h.env
					out, err := c.CombinedOutput()
					results[i] = result{string(out), err}
				}(i, ref)
			}
			wg.Wait()
			for i, r := range results {
				if r.err != nil {
					h.fail("concurrent client %d failed: %v %s", i, r.err, r.out)
				}
				var payload map[string]any
				if err := json.Unmarshal([]byte(r.out), &payload); err != nil || payload["status"] != "ok" {
					h.fail("concurrent client returned invalid result: %s", r.out)
				}
			}
			for _, ref := range []string{"api", "db"} {
				e := h.target.Elements[ref]
				e.Description = "concurrent " + ref
				h.target.Elements[ref] = e
			}
			if h.yamlGraph().canonical() != beforeYAML {
				h.fail("database-only writes touched YAML")
			}
			h.check()
			h.pull(false)
			h.inventory()
			out, _, err := h.runRaw(filepath.Join(h.dir, "db-only"), "text", "", "list", "elements")
			if err != nil || !strings.Contains(out, "api") || !strings.Contains(out, "db") {
				h.fail("database-only text inventory: %v %s", err, out)
			}
			h.check()
		})
	}
}

func TestLifecycleImportRollback(t *testing.T) {
	for _, backend := range []string{"sqlite", "remote", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			if backend == "postgres" && os.Getenv("TLD_TEST_POSTGRES_URL") == "" {
				t.Skip("set TLD_TEST_POSTGRES_URL")
			}
			h := newHarnessFor(t, backend)
			h.add("existing", "root", 0, 0)
			if backend == "postgres" {
				h.sqlExec(`CREATE FUNCTION reject_workflow_element() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.name='z-fail' THEN RAISE EXCEPTION 'injected import failure'; END IF; RETURN NEW; END $$`)
				h.sqlExec(`CREATE TRIGGER workflow_fail BEFORE INSERT ON elements FOR EACH ROW EXECUTE FUNCTION reject_workflow_element()`)
			} else {
				h.sqlExec(`CREATE TRIGGER workflow_fail BEFORE INSERT ON elements WHEN NEW.name='z-fail' BEGIN SELECT RAISE(ABORT,'injected import failure'); END`)
			}
			input := "elements:\n  a-created:\n    name: a-created\n  z-fail:\n    name: z-fail\n"
			// PostgreSQL sequences are not transactional, so semantic rows rather than
			// sequence counters define rollback integrity on that backend.
			before := h.fingerprint()
			h.commandInput(true, input, "import", "-")
			if h.fingerprint() != before {
				h.fail("failed batch leaked rows or changed cache")
			}
			h.check()
			if backend == "postgres" {
				h.sqlExec(`DROP TRIGGER workflow_fail ON elements`)
				h.sqlExec(`DROP FUNCTION reject_workflow_element()`)
			} else {
				h.sqlExec(`DROP TRIGGER workflow_fail`)
			}
			h.importNew("recovered", true)
			h.sync()
			h.pull(false)
		})
	}
}

func TestLifecycleCacheRefreshFailure(t *testing.T) {
	h := newHarnessFor(t, "remote")
	h.add("api", "root", 0, 0)
	h.faults.failWrites.Store(true)
	h.unchanged(true, "update", "element", "api", "description", "must not apply")
	h.faults.failWrites.Store(false)
	h.manual("api", "pending edit to sync", 125, 75)
	before := h.fingerprint()
	h.faults.failExportAfterWrite.Store(true)
	r := h.command(true, "sync")
	detail := r["error"].(map[string]any)
	if detail["code"] != "cache_update_failed" || detail["applied"] != true {
		h.fail("missing partial-application recovery contract: %s", asJSON(r))
	}
	h.target = h.local.clone()
	if before == h.fingerprint() {
		h.fail("fault did not follow a committed target write")
	}
	h.check() // Old sync baseline and pending YAML are legitimate here.
	h.faults.failExportAfterWrite.Store(false)
	h.faults.failExport.Store(false)
	h.pull(true)
	h.sync()
	h.pull(false)
}

func TestLifecycleLocalCacheWriteFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only file permissions cannot inject failure as root")
	}
	h := newHarness(t)
	h.add("api", "root", 0, 0)
	path := filepath.Join(h.workspace, "elements.yaml")
	if err := os.Chmod(path, 0400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })
	r := h.command(true, "update", "element", "api", "description", "target committed")
	detail := r["error"].(map[string]any)
	if detail["code"] != "cache_update_failed" || detail["applied"] != true {
		h.fail("missing cache-write recovery contract: %s", asJSON(r))
	}
	e := h.target.Elements["api"]
	e.Description = "target committed"
	h.target.Elements["api"] = e
	h.check()
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	h.pull(false)
	h.sync()
	h.pull(false)
}

func TestLifecycleInterruptedImport(t *testing.T) {
	h := newHarnessFor(t, "remote")
	h.add("existing", "root", 0, 0)
	before := h.fingerprint()
	h.faults.interrupt.Store(true)
	c := exec.Command(binary, "--workspace", h.dir, "--target", "remote", "--format", "json", "import", "-")
	c.Dir = h.dir
	c.Env = h.env
	c.Stdin = strings.NewReader("elements:\n  a-created:\n    name: a-created\n  interrupt:\n    name: interrupt\n")
	var output bytes.Buffer
	c.Stdout = &output
	c.Stderr = &output
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c.ProcessState == nil {
			_ = c.Process.Kill()
			_ = c.Wait()
		}
	})
	select {
	case <-h.faults.entered:
	case <-time.After(20 * time.Second):
		t.Fatal("import never reached its uncommitted row barrier")
	}
	if err := c.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := c.Wait(); err == nil {
		t.Fatal("interrupted CLI unexpectedly succeeded")
	}
	select {
	case <-h.faults.finished:
	case <-time.After(10 * time.Second):
		t.Fatal("server did not roll back canceled import")
	}
	h.faults.interrupt.Store(false)
	if h.fingerprint() != before {
		h.fail("interrupted transaction leaked state")
	}
	h.check()
	h.importNew("recovered", true)
	h.pull(false)
}
