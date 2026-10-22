import { describe, expect, it, vi } from 'vitest';
import { flushPlanEdits, PlanPanelSaveBarrier, registerPlanSave } from './planSaveParticipants';
import { planSaveQueue } from './planSaveQueue';

describe('handoff save barrier', () => {
  it('waits for an in-flight property save before flushing the latest draft', async () => {
    const barrier = new PlanPanelSaveBarrier();
    const finish = barrier.begin();
    const save = vi.fn(async () => {});
    const pending = barrier.flush(save);
    expect(barrier.blocked).toBe(true);
    await Promise.resolve();
    expect(save).not.toHaveBeenCalled();
    expect(barrier.flush(save)).toBe(pending);
    finish(); await pending;
    expect(save).toHaveBeenCalledTimes(1);
    expect(barrier.blocked).toBe(false);
  });
  it('stops handoff on a failed property save and releases the barrier for retry', async () => {
    const barrier = new PlanPanelSaveBarrier();
    const save = vi.fn().mockRejectedValueOnce(new Error('offline')).mockResolvedValue(undefined);
    const unregister = registerPlanSave('failed-handoff', () => barrier.flush(save));
    const metadata = vi.fn(async () => {});
    planSaveQueue('failed-handoff').set('objective', metadata);
    try {
      await expect(flushPlanEdits('failed-handoff')).rejects.toThrow('offline');
      expect(metadata).not.toHaveBeenCalled();
      expect(barrier.blocked).toBe(false);
      await flushPlanEdits('failed-handoff');
      expect(metadata).toHaveBeenCalledTimes(1);
    } finally { unregister(); }
  });
});
