import { afterEach, describe, expect, it, vi } from "vitest";
import { activePlanId, planFetch, withPlanPath } from "./planContext";

afterEach(() => vi.unstubAllGlobals());
describe("plan request isolation", () => {
  it("keeps plan context across diagram navigation", () => {
    vi.stubGlobal("window", { location: { origin: "https://example.test", pathname: "/views/12", search: "?plan=draft" } });
    expect(withPlanPath("/views/-42?element=-3")).toBe("/views/-42?element=-3&plan=draft");
    expect(withPlanPath("/repositories")).toBe("/repositories");
  });
  it("only activates plan context on editor routes", () => {
    vi.stubGlobal("window", {
      location: { pathname: "/repositories", search: "?plan=draft" },
    });
    expect(activePlanId()).toBe("");
    vi.stubGlobal("window", {
      location: { pathname: "/views/12", search: "?plan=draft" },
    });
    expect(activePlanId()).toBe("draft");
  });
  it("preserves request authentication and carries the observed revision to writes", async () => {
    vi.stubGlobal("window", {
      location: { pathname: "/views/12", search: "?plan=revision-test" },
    });
    const fetch = vi
      .fn()
      .mockResolvedValue(
        new Response("{}", { headers: { "X-Tld-Plan-Revision": "7" } }),
      );
    vi.stubGlobal("fetch", fetch);
    await planFetch(
      new Request("https://example.test/api", {
        headers: { Authorization: "Bearer test" },
      }),
    );
    expect(fetch.mock.calls[0][1].headers.get("Authorization")).toBe(
      "Bearer test",
    );
    await planFetch("/api", { method: "POST" });
    expect(fetch.mock.calls[1][1].headers.get("X-Tld-Plan-Revision")).toBe("7");
    expect(fetch.mock.calls[1][1].headers.get("X-Tld-Plan")).toBe(
      "revision-test",
    );
    vi.stubGlobal("window", {
      location: { pathname: "/views/12", search: "" },
    });
    await planFetch("/api", { method: "POST" });
    expect(fetch.mock.calls[2][1].headers.has("X-Tld-Plan")).toBe(false);
  });
});
