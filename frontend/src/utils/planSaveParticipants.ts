import { planSaveQueue } from './planSaveQueue';

const participants = new Map<string, Set<() => Promise<void>>>();
export function registerPlanSave(id: string, save: () => Promise<void>) {
  let saves = participants.get(id);
  if (!saves) { saves = new Set(); participants.set(id, saves); }
  saves.add(save);
  return () => { saves.delete(save); if (!saves.size) participants.delete(id); };
}
export async function flushPlanEdits(id: string) {
  for (const save of [...(participants.get(id) ?? [])]) await save();
  await planSaveQueue(id).flush();
}

/** Stops debounced saves from overtaking an explicit handoff flush. */
export class PlanPanelSaveBarrier {
  blocked = false;
  private active = 0;
  private waiters: Array<() => void> = [];
  private flushing?: Promise<void>;
  begin() {
    this.active++;
    return () => {
      this.active--;
      if (!this.active) this.waiters.splice(0).forEach(resolve => resolve());
    };
  }
  flush(save: () => Promise<void>): Promise<void> {
    if (this.flushing) return this.flushing;
    this.blocked = true;
    const wait = this.active ? new Promise<void>(resolve => this.waiters.push(resolve)) : Promise.resolve();
    this.flushing = wait.then(save).finally(() => { this.blocked = false; this.flushing = undefined; });
    return this.flushing;
  }
}
