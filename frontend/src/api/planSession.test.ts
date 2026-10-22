import { describe, expect, it } from 'vitest';
import { knownPlanRevision, observePlanRevision, queuePlanMutation } from './planSession';

describe('plan revision coordination', () => {
  it('does not let late reads lower the revision used for writes', () => {
    observePlanRevision('late-read', 8);
    observePlanRevision('late-read', 3);
    expect(knownPlanRevision('late-read')).toBe(8);
  });
  it('serializes writes and lets another plan progress independently', async () => {
    let release!: () => void;
    const gate = new Promise<void>(resolve => { release = resolve; });
    const calls: string[] = [];
    const first = queuePlanMutation('serial', async () => { calls.push('first'); await gate; observePlanRevision('serial', 2); });
    const second = queuePlanMutation('serial', async () => { calls.push(`second:${knownPlanRevision('serial')}`); });
    await queuePlanMutation('independent', async () => { calls.push('other'); });
    expect(calls).toEqual(['first', 'other']);
    release(); await Promise.all([first, second]);
    expect(calls).toEqual(['first', 'other', 'second:2']);
  });
  it('allows a retry after a rejected write', async () => {
    await expect(queuePlanMutation('retry', async () => { throw new Error('conflict'); })).rejects.toThrow('conflict');
    await expect(queuePlanMutation('retry', async () => 'saved')).resolves.toBe('saved');
  });
});
