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
import { knownPlanRevision, observePlanRevision, queuePlanMutation } from "./planSession";
import { Code, ConnectError } from "@connectrpc/connect";
import type { UpdatePlanRequest } from "@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/plan_service_pb";
export const plansClient = createClient(PlanService, transport);
export type { Plan, PlanProjectedResource, PlanResource };
export type PlanOperationInput = MessageInitShape<typeof PlanOperationSchema>;
export function planChanged(planId: string) {
  window.dispatchEvent(new CustomEvent("tld:plan-changed", { detail: planId }));
}

async function mutatePlan<T>(planId: string, work: (revision: number) => Promise<T>) {
  return queuePlanMutation(planId, async () => {
    let revision = knownPlanRevision(planId);
    if (revision === undefined) {
      const { plan } = await plansClient.getPlan({ planId });
      if (!plan) throw new Error("Plan not found");
      revision = plan.revision;
      observePlanRevision(planId, revision);
    }
    try { return await work(revision); } catch (e) {
      if (ConnectError.from(e).code === Code.Aborted) {
        const fresh = await plansClient.getPlan({ planId });
        if (fresh.plan) observePlanRevision(planId, fresh.plan.revision);
        throw new Error("This plan changed in another window. Your input is kept. Review the latest plan and retry saving.");
      }
      throw e;
    }
  });
}

export function appendPlan(planId: string, operations: PlanOperationInput[]) {
  const requestId = crypto.randomUUID();
  return mutatePlan(planId, async revision => {
    const request = { planId, expectedRevision: revision, requestId,
      operations: operations.map(op => create(PlanOperationSchema, op)) };
    const send = () => plansClient.appendOperations(request);
    const result = await send().catch(e => {
      // A lost response may follow a committed write. Retry the same identity.
      if (ConnectError.from(e).code === Code.Unavailable) return send();
      throw e;
    });
    if (result.plan) observePlanRevision(planId, result.plan.revision);
    planChanged(planId);
    return result;
  });
}

export function updatePlan(planId: string, fields: Pick<UpdatePlanRequest, "name" | "objective" | "status">) {
  return mutatePlan(planId, async revision => {
    const result = await plansClient.updatePlan({ planId, expectedRevision: revision, ...fields }).catch(async e => {
      if (ConnectError.from(e).code === Code.Unavailable) {
        const fresh = await plansClient.getPlan({ planId });
        if (fresh.plan) {
          observePlanRevision(planId, fresh.plan.revision);
          if (Object.entries(fields).every(([field, value]) => value === undefined || fresh.plan![field as keyof Plan] === value)) return fresh;
        }
      }
      throw e;
    });
    if (result.plan) observePlanRevision(planId, result.plan.revision);
    planChanged(planId);
    return result;
  });
}

export function startPlanImplementation(planId: string) {
  return mutatePlan(planId, async revision => {
    const result = await plansClient.lockImplementation({ planId, expectedRevision: revision });
    const fresh = await plansClient.getPlan({ planId });
    if (fresh.plan) observePlanRevision(planId, fresh.plan.revision);
    planChanged(planId);
    return result;
  });
}
export async function planProjection(
  planId: string,
  fullWorkspace = false,
  radius = 0,
  revision?: number,
) {
  const first = await plansClient.getProjection({
    planId,
    fullWorkspace,
    radius,
    revision,
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
  observePlanRevision(planId, first.revision);
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
  const revision = knownPlanRevision(planId);
  if (revision === undefined) throw new Error("Reload the plan before editing");
  return revision;
}
