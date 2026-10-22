import { useEffect, useState } from 'react';
import { Box, Button, Drawer, DrawerBody, DrawerCloseButton, DrawerContent, DrawerHeader, HStack, Select, Text, useToast } from '@chakra-ui/react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { api, type IndexedRepository } from '../api/client';
import { appendPlan, plansClient, planProjection, resourceId, type Plan } from '../api/plans';
import { flushPlanEdits } from '../utils/planSaveParticipants';
import RepositoryTargetPicker from './RepositoryTargetPicker';
import RepositoryPlans from './RepositoryPlans';

export default function PlanControls({ viewId, elementIds = [], connectorIds = [] }: {
  viewId: number | null; elementIds?: number[]; connectorIds?: number[];
}) {
  const [params, setParams] = useSearchParams();
  const id = params.get('plan') ?? '';
  const [plan, setPlan] = useState<Plan>();
  const [repositories, setRepositories] = useState<IndexedRepository[]>([]);
  const [repository, setRepository] = useState(params.get('repo') ?? '');
  const [picking, setPicking] = useState(false);
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [role, setRole] = useState('change');
  const toast = useToast();
  const navigate = useNavigate();
  useEffect(() => {
    let active = true;
    const refresh = () => {
      if (!id) { setPlan(undefined); return; }
      void plansClient.getPlan({ planId: id }).then(r => { if (active) setPlan(r.plan); })
        .catch(e => { if (active) toast({ title: String(e), status: 'error' }); });
    };
    refresh();
    window.addEventListener('tld:plan-changed', refresh);
    return () => { active = false; window.removeEventListener('tld:plan-changed', refresh); };
  }, [id, toast]);
  const perform = async (work: () => Promise<void>) => {
    if (busy) return;
    setBusy(true);
    try { await work(); } catch (e) { toast({ title: String(e), status: 'error' }); }
    finally { setBusy(false); }
  };
  const start = (resume: boolean) => perform(async () => {
    let repo = repository;
    if (!repo) {
      const list = await api.repositories.list();
      setRepositories(list);
      if (list.length === 1) { repo = list[0].id; setRepository(repo); }
      else { setPicking(true); return; }
    }
    const response = await plansClient.createPlan({ repositoryId: repo, resume });
    if (!response.plan) return;
    setParams(old => { const next = new URLSearchParams(old); next.set('plan', response.plan!.id); next.set('repo', repo); return next; });
    setOpen(true);
  });
  const include = () => perform(async () => {
    await flushPlanEdits(id);
    const projection = await planProjection(id, true);
    const matches = projection.resources.filter(v => {
      const r = v.resource;
      return r && ((r.value.case === 'element' && elementIds.includes(resourceId(r))) ||
        (r.value.case === 'connector' && connectorIds.includes(resourceId(r))) ||
        (!elementIds.length && !connectorIds.length && r.value.case === 'view' && resourceId(r) === viewId));
    });
    if (!matches.length) { toast({ title: 'Select diagram objects to add to the plan', status: 'info' }); return; }
    await appendPlan(id, matches.map(v => ({ action: { case: 'include', value: { resourceKey: v.resource!.key, role, explicit: true } } })));
    setOpen(true);
  });
  const close = () => { void perform(async () => { await flushPlanEdits(id); setOpen(false); }); };
  if (!id) return <HStack flexWrap="wrap">
    {picking && <RepositoryTargetPicker value={repository} groups={[{ options: repositories.map(r => ({ value: r.id, label: r.name || r.root })) }]} onChange={setRepository} aria-label="Plan repository" />}
    <Button size="xs" isDisabled={busy} onClick={() => void start(false)}>New plan</Button>
    <Button size="xs" variant="ghost" isDisabled={busy} onClick={() => void start(true)}>Continue draft</Button>
  </HStack>;
  return <>
    <HStack flexWrap="wrap" justify="center" gap={1} minW={0}>
      <Button size="xs" variant="ghost" maxW="220px" overflow="hidden" textOverflow="ellipsis" onClick={() => setOpen(true)}>Plan · {plan?.name ?? 'Loading…'}</Button>
      <Select size="xs" w="140px" aria-label="Plan role" value={role} onChange={e => setRole(e.target.value)}>
        <option value="change">Change</option><option value="preserve">Keep unchanged</option><option value="context">Reference</option>
      </Select>
      <Button size="xs" isDisabled={busy || ['completed', 'archived'].includes(plan?.status ?? '')} onClick={() => void include()}>Add {elementIds.length || connectorIds.length ? 'selection' : 'view'}</Button>
      <Button size="xs" isDisabled={busy} onClick={() => void perform(async () => {
        await flushPlanEdits(id);
        setParams(old => { const next = new URLSearchParams(old); next.delete('plan'); return next; });
      })}>Exit plan</Button>
    </HStack>
    <Drawer isOpen={open} placement="right" size={{ base: 'full', md: 'md' }} onClose={close} blockScrollOnMount={false} trapFocus={false}>
      <DrawerContent containerProps={{ pointerEvents: 'none' }} pointerEvents="auto" maxW={{ base: '100vw', md: '460px' }}>
        <DrawerCloseButton /><DrawerHeader pb={1}>Change plan</DrawerHeader>
        <DrawerBody p={0}>
          <Box px={4}><Text fontSize="xs" color="gray.500">Keep the plan open while selecting objects and editing the architecture.</Text></Box>
          {plan && <RepositoryPlans repositoryId={plan.repositoryId} compact onWatch={() => navigate(`/repositories?repo=${encodeURIComponent(plan.repositoryId)}&mode=watch&plan=${encodeURIComponent(id)}`)} />}
        </DrawerBody>
      </DrawerContent>
    </Drawer>
  </>;
}
