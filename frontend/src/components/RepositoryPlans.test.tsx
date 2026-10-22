import React from 'react';
import { act, create as render, type ReactTestRenderer } from 'react-test-renderer';
import { create } from '@bufbuild/protobuf';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PlanSchema, PlanProjectedResourceSchema, ImplementationResponseSchema } from '@buf/tldiagramcom_diagram.bufbuild_es/diag/v1/plan_service_pb';
import RepositoryPlans from './RepositoryPlans';

const mocks = vi.hoisted(() => ({
  params: new URLSearchParams(), setParams: vi.fn(), projection: vi.fn(), prompt: vi.fn(), checklist: vi.fn(), list: vi.fn(),
  update: vi.fn(), start: vi.fn(), watch: vi.fn(), watchStatus: vi.fn(), capabilities: vi.fn(), append: vi.fn(), review: vi.fn(),
  implementation: {} as Record<string, unknown>, calls: [] as string[],
}));
vi.mock('react-router-dom', () => ({ useNavigate: () => vi.fn(), useSearchParams: () => [mocks.params, mocks.setParams] }));
vi.mock('@chakra-ui/icons', () => ({ SmallCloseIcon: () => null }));
vi.mock('@chakra-ui/react', async () => {
  const ReactModule = await import('react');
  const box = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('div', props, children);
  const button = ({ children, ...props }: { children?: React.ReactNode }) => ReactModule.createElement('button', props, children);
  const input = (props: Record<string, unknown>) => ReactModule.createElement('input', props);
  return { Box: box, Button: button, HStack: box, Input: input, IconButton: button, Select: box, Text: box, Textarea: input, VStack: box, useToast: () => mocks.review };
});
vi.mock('./ElementPanel', () => ({ default: () => null }));
vi.mock('./ConnectorPanel', () => ({ default: () => null }));
vi.mock('./ViewMarkdownPanel/MarkdownEditor', () => ({ MarkdownEditor: () => null }));
vi.mock('../api/client', () => ({ api: { system: { capabilities: mocks.capabilities }, repositories: { startWatch: mocks.watch, watchStatus: mocks.watchStatus } }, protoElementToLibrary: () => ({}), protoConnector: () => ({}) }));
vi.mock('../api/plans', () => ({
  plansClient: { listPlans: mocks.list, getPrompt: mocks.prompt, getChecklist: mocks.checklist },
  planProjection: mocks.projection, updatePlan: mocks.update, startPlanImplementation: mocks.start, appendPlan: mocks.append,
  resourceLabel: (resource: { value: { value: { name: string } } }) => resource.value.value.name,
  resourceId: () => 1,
}));
vi.mock('../hooks/usePlanImplementation', () => ({ usePlanImplementation: () => mocks.implementation }));

let renderer: ReactTestRenderer | undefined;
let specification = create(PlanSchema);
function text() { return renderer!.root.findAll(node => typeof node.type === 'string').map(node => [...node.children.filter(child => typeof child === 'string'), typeof node.props.value === 'string' ? node.props.value : ''].join(' ')).join(' '); }
function button(label: string) {
  return renderer!.root.findAllByType('button').find(node => node.children.join('') === label)!;
}
beforeEach(() => {
  vi.clearAllMocks();
  mocks.calls = [];
  specification = create(PlanSchema, { id: crypto.randomUUID(), repositoryId: 'repo', name: 'Feature', objective: 'Ship retries', revision: 2, status: 'draft' });
  mocks.params = new URLSearchParams({ plan: specification.id });
  mocks.implementation = { state: create(ImplementationResponseSchema), busy: false, error: '', message: '', context: [], review: mocks.review };
  const resource = create(PlanProjectedResourceSchema, { resource: { key: 'element:1', value: { case: 'element', value: { id: 1, name: 'API' } } }, inclusionReason: 'member', role: 'change' });
  mocks.projection.mockImplementation(async () => ({ plan: specification, revision: specification.revision, resources: [resource], operations: [] }));
  mocks.list.mockImplementation(async () => ({ plans: [specification] }));
  mocks.prompt.mockResolvedValue({ text: 'Pinned prompt' }); mocks.checklist.mockResolvedValue({ items: [] });
  mocks.capabilities.mockResolvedValue({ watch: false }); mocks.watch.mockResolvedValue({}); mocks.watchStatus.mockResolvedValue({ running: false });
  mocks.update.mockImplementation(async (_id: string, fields: Record<string, string>) => {
    mocks.calls.push('save'); specification = { ...specification, ...fields, revision: specification.revision + 1 };
  });
  mocks.start.mockImplementation(async () => {
    mocks.calls.push('start'); specification = { ...specification, status: 'ready', revision: specification.revision + 1 };
    mocks.implementation = { ...mocks.implementation, state: create(ImplementationResponseSchema, { locked: true, planId: specification.id, repositoryId: 'repo' }), plan: specification };
  });
  const events = new EventTarget();
  vi.stubGlobal('window', events);
});
afterEach(async () => { if (renderer) await act(async () => renderer!.unmount()); renderer = undefined; vi.unstubAllGlobals(); });

describe('guided plan workflow', () => {
  it('flushes input before freezing and supports implementation without Watch', async () => {
    await act(async () => { renderer = render(<RepositoryPlans repositoryId="repo" onWatch={vi.fn()} />); });
    expect(text()).toContain('Diagram changes save immediately');
    await act(async () => {
      renderer!.root.findAllByType('input').find(node => node.props['aria-label'] === 'Plan objective')!.props.onChange({ target: { value: 'Latest objective' } });
    });
    await act(async () => { await button('Start implementation').props.onClick(); });
    expect(mocks.calls).toEqual(['save', 'start']);
    expect(mocks.watch).not.toHaveBeenCalled();
    expect(text()).toContain('Finish implementation');
    expect(button('Copy prompt').props.isDisabled).toBe(false);
    expect(mocks.prompt).toHaveBeenLastCalledWith({ planId: specification.id, revision: specification.revision });
  });
  it('keeps review and recovery available when Watch startup fails', async () => {
    mocks.capabilities.mockResolvedValue({ watch: true }); mocks.watch.mockRejectedValue(new Error('watch offline'));
    await act(async () => { renderer = render(<RepositoryPlans repositoryId="repo" onWatch={vi.fn()} />); });
    await act(async () => { await button('Start implementation').props.onClick(); });
    expect(text()).toContain('Implementation prepared; Watch unavailable');
    expect(button('Retry Watch')).toBeDefined();
    expect(button('End without applying')).toBeDefined();
    expect(text()).toContain('specification is frozen');
  });
  it('reuses a running watcher when preparing implementation', async () => {
    mocks.capabilities.mockResolvedValue({ watch: true }); mocks.watchStatus.mockResolvedValue({ running: true });
    await act(async () => { renderer = render(<RepositoryPlans repositoryId="repo" onWatch={vi.fn()} />); });
    await act(async () => { await button('Start implementation').props.onClick(); });
    expect(mocks.start).toHaveBeenCalledTimes(1);
    expect(mocks.watch).not.toHaveBeenCalled();
    expect(text()).not.toContain('Watch unavailable');
  });
  it('does not freeze while a save fails and retains the entered objective for retry', async () => {
    mocks.update.mockRejectedValueOnce(new Error('offline'));
    await act(async () => { renderer = render(<RepositoryPlans repositoryId="repo" onWatch={vi.fn()} />); });
    await act(async () => { renderer!.root.findAllByType('input').find(node => node.props['aria-label'] === 'Plan objective')!.props.onChange({ target: { value: 'Keep this goal' } }); });
    await act(async () => { await button('Start implementation').props.onClick(); });
    expect(mocks.start).not.toHaveBeenCalled();
    expect(text()).toContain('Keep this goal');
    expect(button('Retry saving')).toBeDefined();
    await act(async () => { await button('Retry saving').props.onClick(); });
    expect(mocks.update).toHaveBeenCalledTimes(2);
    expect(specification.objective).toBe('Keep this goal');
  });
  it('ignores an old plan response after navigating to another plan', async () => {
    let resolveOld!: (value: unknown) => void;
    const oldID = specification.id;
    mocks.projection.mockImplementationOnce(() => new Promise(resolve => { resolveOld = resolve; }));
    await act(async () => { renderer = render(<RepositoryPlans repositoryId="repo" onWatch={vi.fn()} />); });
    specification = { ...specification, id: 'new-plan', name: 'New feature' };
    mocks.params = new URLSearchParams({ plan: specification.id });
    await act(async () => { renderer!.update(<RepositoryPlans repositoryId="repo" onWatch={vi.fn()} />); });
    await act(async () => { resolveOld({ plan: { ...specification, id: oldID, name: 'Old feature' }, revision: 2, resources: [], operations: [] }); });
    expect(text()).toContain('New feature');
    expect(text()).not.toContain('Old feature');
  });
});
