import { describe, expect, it, vi } from 'vitest';
import { PlanSaveQueue } from './planSaveQueue';

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>(done => { resolve = done; });
  return { promise, resolve };
}
describe('plan saving', () => {
  it('shares overlapping flushes and saves newer input after an in-flight save', async () => {
    const queue = new PlanSaveQueue();
    const gate = deferred();
    const old = vi.fn(() => gate.promise);
    const latest = vi.fn(async () => {});
    queue.set('objective', old, 'old');
    const first = queue.flush();
    queue.set('objective', latest, 'latest');
    expect(queue.flush()).toBe(first);
    expect(queue.draft('objective')).toBe('latest');
    gate.resolve();
    await first;
    expect(old).toHaveBeenCalledTimes(1);
    expect(latest).toHaveBeenCalledTimes(1);
    expect(queue.size).toBe(0);
  });
  it('keeps failed input and retries without repeating already saved fields', async () => {
    const queue = new PlanSaveQueue();
    const name = vi.fn(async () => {});
    const objective = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue(undefined);
    queue.set('name', name, 'Name');
    queue.set('objective', objective, 'Goal');
    await expect(queue.flush()).rejects.toThrow('offline');
    expect(queue.draft('objective')).toBe('Goal');
    expect(queue.error).toContain('offline');
    await queue.flush();
    expect(name).toHaveBeenCalledTimes(1);
    expect(objective).toHaveBeenCalledTimes(2);
    expect(queue.size).toBe(0);
    expect(queue.error).toBe('');
  });
});
