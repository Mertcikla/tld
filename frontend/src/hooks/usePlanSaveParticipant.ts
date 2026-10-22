import { useEffect, useRef } from 'react';
import { activePlanId } from '../api/planContext';
import { PlanPanelSaveBarrier, registerPlanSave } from '../utils/planSaveParticipants';

export function usePlanSaveParticipant(planId: string | undefined, enabled: boolean, save: () => Promise<void>) {
  const id = planId ?? activePlanId();
  const barrier = useRef(new PlanPanelSaveBarrier()).current;
  const latest = useRef(save);
  latest.current = save;
  useEffect(() => {
    if (!id || !enabled) return;
    return registerPlanSave(id, () => barrier.flush(() => latest.current()));
  }, [id, enabled, barrier]);
  return barrier;
}
