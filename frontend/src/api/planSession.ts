// Shared by the editor transport and plan controls so every write observes the
// preceding write's revision. Reads may finish late and must not lower it.
const revisions = new Map<string, number>();
const queues = new Map<string, Promise<unknown>>();
export function observePlanRevision(id: string, revision: number) {
  if (!Number.isSafeInteger(revision) || revision < 0) return;
  revisions.set(id, Math.max(revisions.get(id) ?? 0, revision));
}
export function knownPlanRevision(id: string) { return revisions.get(id); }
export function queuePlanMutation<T>(id: string, work: () => Promise<T>): Promise<T> {
  const run = (queues.get(id) ?? Promise.resolve()).then(work);
  const tail = run.catch(() => undefined);
  queues.set(id, tail);
  void tail.then(() => { if (queues.get(id) === tail) queues.delete(id); });
  return run;
}
