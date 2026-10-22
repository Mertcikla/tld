import { createClient } from "@connectrpc/connect";
import { create, type MessageInitShape } from "@bufbuild/protobuf";
import {
  PlanService,
  PlanOperationSchema,
  type Plan,
  type PlanProjectedResource,
  type PlanResource,
} from "@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/plan_service_pb";
import { transport } from "./transport";
export const plansClient = createClient(PlanService, transport);
export type { Plan, PlanProjectedResource, PlanResource };
export type PlanOperationInput = MessageInitShape<typeof PlanOperationSchema>;
const revisions = new Map<string, number>();
const queues = new Map<string, Promise<unknown>>();
export function appendPlan(planId: string, operations: PlanOperationInput[]) {
  const previous = queues.get(planId) ?? Promise.resolve();
  const requestId = crypto.randomUUID();
  const run = previous.then(async () => {
    let revision = revisions.get(planId);
    if (revision === undefined) {
      const { plan } = await plansClient.getPlan({ planId });
      if (!plan) throw new Error("Plan not found");
      revision = plan.revision;
    }
    const result = await plansClient.appendOperations({
      planId,
      expectedRevision: revision,
      requestId,
      operations: operations.map((op) => create(PlanOperationSchema, op)),
    });
    if (result.plan) revisions.set(planId, result.plan.revision);
    window.dispatchEvent(
      new CustomEvent("tld:plan-changed", { detail: planId }),
    );
    return result;
  });
  queues.set(
    planId,
    run.catch(() => undefined),
  );
  return run;
}
export async function planProjection(
  planId: string,
  fullWorkspace = false,
  radius = 0,
) {
  const first = await plansClient.getProjection({
    planId,
    fullWorkspace,
    radius,
  });
  let cursor = first.nextCursor;
  while (cursor) {
    const page = await plansClient.getProjection({
      planId,
      revision: first.revision,
      fullWorkspace,
      radius,
      cursor,
    });
    first.resources.push(...page.resources);
    cursor = page.nextCursor;
  }
  revisions.set(planId, first.revision);
  return first;
}
export function resourceLabel(r: PlanResource, resources: PlanResource[] = []): string {
  const name = (kind: 'element' | 'view', id: number) => {
    const resource = resources.find(item => item.value.case === kind && resourceId(item) === id);
    return resource ? resourceLabel(resource) : `Missing ${kind}`;
  };
  if (r.value.case === 'connector') {
    const c = r.value.value;
    const arrow = c.direction === 'backward' ? ' ← ' : c.direction === 'both' ? ' ↔ ' : c.direction === 'none' ? ' — ' : ' → ';
    return name('element', c.sourceElementId) + arrow + name('element', c.targetElementId) + (c.label?.trim() ? ` (${c.label.trim()})` : '');
  }
  if (r.value.case === 'placement') {
    return `${name('element', r.value.value.elementId)} in ${name('view', r.value.value.viewId)}`;
  }
  const v = r.value.value;
  if (v && 'name' in v && v.name.trim()) return v.name;
  return `Unnamed ${r.value.case ?? 'object'}`;
}
export function resourceId(r: PlanResource): number {
  return r.value.value?.id ?? 0;
}

export function observedPlanRevision(planId: string): number {
  const revision = revisions.get(planId);
  if (revision === undefined) throw new Error("Reload the plan before editing");
  return revision;
}
