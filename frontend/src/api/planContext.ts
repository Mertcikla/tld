/** A URL-scoped plan context cannot leak into ordinary workspace navigation. */
export function activePlanId(): string {
  if (
    typeof window === "undefined" ||
    !/\/views\/[^/]+/.test(window.location.pathname)
  )
    return "";
  return new URLSearchParams(window.location.search).get("plan") ?? "";
}
const revisions = new Map<string, string>();
export async function planFetch(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<Response> {
  const headers = new Headers(
    input instanceof Request ? input.headers : undefined,
  );
  new Headers(init?.headers).forEach((value, key) => headers.set(key, value));
  const plan = activePlanId();
  if (plan) {
    headers.set("X-Tld-Plan", plan);
    const revision = revisions.get(plan);
    if (revision) headers.set("X-Tld-Plan-Revision", revision);
    headers.set("X-Tld-Request-Id", crypto.randomUUID());
  }
  const result = await fetch(input, { ...init, headers });
  const observed = result.headers.get("X-Tld-Plan-Revision");
  if (plan && observed && result.ok) revisions.set(plan, observed);
  return result;
}

/** Carry planning context through drill-down/up navigation inside the editor. */
export function withPlanPath(path: string): string {
  const plan = activePlanId();
  if (!plan || !path.startsWith('/views/')) return path;
  const url = new URL(path, window.location.origin);
  url.searchParams.set('plan', plan);
  return url.pathname + url.search + url.hash;
}
