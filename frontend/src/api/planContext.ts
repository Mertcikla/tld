import { knownPlanRevision, observePlanRevision, queuePlanMutation } from './planSession';

/** A URL-scoped plan context cannot leak into ordinary workspace navigation. */
export function activePlanId(): string {
  if (typeof window === 'undefined' || !/\/views\/[^/]+/.test(window.location?.pathname ?? '')) return '';
  return new URLSearchParams(window.location.search).get('plan') ?? '';
}
export async function planFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const plan = activePlanId();
  const url = input instanceof Request ? input.url : String(input);
  // PlanService has its own mutation coordinator. Re-queueing it here would
  // deadlock the queue and mix plan revisions with implementation revisions.
  const planService = url.includes('/diag.v1.PlanService/');
  const requestId = plan && !planService ? crypto.randomUUID() : '';
  const send = async () => {
    const headers = new Headers(input instanceof Request ? input.headers : undefined);
    new Headers(init?.headers).forEach((value, key) => headers.set(key, value));
    if (plan && !planService) {
      headers.set('X-Tld-Plan', plan);
      const revision = knownPlanRevision(plan);
      if (revision !== undefined) headers.set('X-Tld-Plan-Revision', String(revision));
      headers.set('X-Tld-Request-Id', requestId);
    }
    const result = await fetch(input, { ...init, headers });
    const observed = result.headers.get('X-Tld-Plan-Revision');
    if (plan && !planService && observed && (result.ok || result.status === 409)) observePlanRevision(plan, Number(observed));
    return result;
  };
  const workspaceWrite = url.includes('/diag.v1.WorkspaceService/') && !/\/(Get|List|Export)[^/]*$/.test(url);
  return plan && workspaceWrite ? queuePlanMutation(plan, send) : send();
}

/** Carry planning context through drill-down/up navigation inside the editor. */
export function withPlanPath(path: string): string {
  const plan = activePlanId();
  if (!plan || !path.startsWith('/views/')) return path;
  const url = new URL(path, window.location.origin);
  url.searchParams.set('plan', plan);
  return url.pathname + url.search + url.hash;
}
