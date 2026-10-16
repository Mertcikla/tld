//go:build cliworkflow

package cliworkflow

import "testing"

func TestLifecycleSharedServiceRetirement(t *testing.T) {
	h := newHarness(t)
	h.add("platform", "root", 0, 0)
	h.createView("platform")
	h.add("operations", "root", 600, 0)
	h.createView("operations")
	h.add("api", "platform", 120, 80)
	h.add("db", "platform", 400, 80)
	h.share("api", "operations", 60, 100)
	key := h.connect("platform", "api", "db", "reads")
	h.pull(false)
	h.manual("api", "manually documented API", 250, -40)
	h.unchanged(false, "sync", "--dry-run")
	h.sync()
	h.sync() // No duplicate elements, placements, or connectors on retry.
	h.pull(false)
	h.relabel(key, "reads and writes")
	h.rename("platform", "system") // Cascade view, placements, and connector keys.
	key = "system/api~db/reads and writes"
	h.unchanged(true, "view", "unplace", "system", "api")
	h.disconnect(key)
	h.unplace("api", "system")
	h.update("api", "technology", "rust")
	h.pull(false) // The operations placement must survive.
	h.unplace("api", "operations")
	h.remove("api")
	h.sync()
	h.pull(false) // Deleted resources must not be resurrected.
	h.pull(false)
}

func TestLifecycleManualEditsProtectionAndRecovery(t *testing.T) {
	h := newHarness(t)
	h.add("platform", "root", 0, 0)
	h.createView("platform")
	h.add("api", "platform", 100, 100)
	h.add("db", "platform", 400, 100)
	h.connect("platform", "api", "db", "reads")
	h.manual("api", "pending YAML change", -80, 220)
	h.reject("local_changes", "pull")
	h.reject("cache_out_of_sync", "update", "element", "db", "description", "must not apply")
	h.unchanged(false, "pull", "--dry-run")
	h.unchanged(false, "sync", "--dry-run")
	h.rename("api", "backend") // Alias changes must preserve the pending edit.
	h.reject("local_changes", "pull")
	h.sync()
	h.update("backend", "description", "") // Clearing must survive reconciliation.
	h.update("backend", "tags", "service,reviewed")
	h.pull(false)
	h.manual("backend", "discard this edit", 800, 900)
	h.pull(true)
	h.unchanged(true, "remove", "element", "db", "--dry-run")
	h.unchanged(false, "remove", "connector", "platform/backend~db/reads", "--dry-run")
	h.unchanged(false, "list", "elements", "--source", "live")
	h.sync()
	h.pull(false)
}

func TestLifecycleExternalChangesAndViewReuse(t *testing.T) {
	h := newHarness(t)
	h.add("platform", "root", 0, 0)
	h.createView("platform")
	h.add("api", "platform", 100, 100)
	h.add("db", "root", 500, 100)
	// Off-view endpoints are valid; reconciliation must not invent a placement.
	key := h.connect("platform", "api", "db", "reads")
	h.externalDescription("api", "changed by another client")
	h.unchanged(false, "pull", "--dry-run")
	h.pull(false)
	h.manual("api", "local pending description", 10, 20)
	h.externalDescription("db", "independent target change")
	h.reject("local_changes", "pull")
	h.pull(true)
	h.disconnect(key)
	h.unplace("api", "platform")
	h.deleteView("platform")
	h.pull(false)
	h.createView("platform")
	h.share("api", "platform", 70, 90)
	h.connect("platform", "api", "db", "reads")
	h.update("api", "description", "")
	h.externalDescription("api", "new target description after a clear")
	h.sync() // An omitted YAML description preserves the new target value.
	h.sync()
	h.pull(false)
	h.pull(false)
}

func TestLifecyclePullAfterRefRename(t *testing.T) {
	for _, when := range []string{"before-rename", "after-rename"} {
		t.Run(when, func(t *testing.T) {
			h := newHarness(t)
			h.add("platform", "root", 0, 0)
			h.createView("platform")
			h.add("api", "platform", 100, 100)
			h.add("db", "platform", 400, 100)
			h.connect("platform", "api", "db", "reads")
			if when == "before-rename" {
				h.externalDescription("api", "changed before alias rename")
			}
			h.rename("platform", "system")
			h.rename("api", "backend")
			if when == "after-rename" {
				h.externalDescription("backend", "changed after alias rename")
			}
			h.externalConnectorDescription("system/backend~db/reads", "changed after selector rename")
			h.unchanged(false, "pull", "--dry-run")
			h.pull(false)
			h.pull(false)
			h.sync()
			h.pull(false)
		})
	}
}

func TestLifecyclePullAfterAppliedUpdate(t *testing.T) {
	h := newHarness(t)
	h.add("platform", "root", 0, 0)
	h.createView("platform")
	h.add("api", "platform", 100, 100)
	h.add("db", "platform", 400, 100)
	key := h.connect("platform", "api", "db", "reads")
	for _, ref := range []string{"api", "platform"} {
		h.update(ref, "description", "already applied CLI description")
		h.externalDescription(ref, "later external description")
		h.pull(false) // The committed CLI update is not an unapplied local edit.
	}
	h.updateConnectorDescription(key, "already applied connector description")
	h.externalConnectorDescription(key, "later external connector description")
	h.pull(false)
	h.pull(false)
	h.sync()
}
