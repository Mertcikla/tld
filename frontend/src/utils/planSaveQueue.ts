/** Coalesces input by field, keeps failed jobs for retry, and never overlaps flushes. */
export class PlanSaveQueue {
  private jobs = new Map<string, () => Promise<void>>();
  private drafts = new Map<string, string>();
  private active?: Promise<void>;
  private listeners = new Set<() => void>();
  private version = 0;
  error = '';
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  snapshot = () => this.version;
  private changed() { this.version++; this.listeners.forEach(listener => listener()); }
  has(key: string) { return this.jobs.has(key); }
  draft(key: string) { return this.drafts.get(key); }
  get size() { return this.jobs.size; }
  set(key: string, job: () => Promise<void>, value?: string) {
    this.jobs.set(key, job);
    if (value !== undefined) this.drafts.set(key, value);
    this.changed();
  }
  flush(): Promise<void> {
    if (this.active) return this.active;
    const run = async () => {
      this.error = '';
      while (this.jobs.size) {
        const [key, job] = this.jobs.entries().next().value!;
        await job();
        if (this.jobs.get(key) === job) { this.jobs.delete(key); this.drafts.delete(key); }
        this.changed();
      }
    };
    this.active = run().catch(e => { this.error = String(e); this.changed(); throw e; })
      .finally(() => { this.active = undefined; this.changed(); });
    return this.active;
  }
}
// Keep in-flight input when closing the drawer or moving between plan screens.
const sessions = new Map<string, PlanSaveQueue>();
export function planSaveQueue(id: string) {
  let queue = sessions.get(id);
  if (!queue) { queue = new PlanSaveQueue(); sessions.set(id, queue); }
  return queue;
}
