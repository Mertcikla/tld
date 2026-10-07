import { test } from 'node:test';
import assert from 'node:assert/strict';
import { limits, validateResult, commentBody, publicationContext, upsertComment, marker } from './lib.mjs';

const head = 'a'.repeat(40);
const base = 'b'.repeat(40);
const trusted = { repository: 'owner/repo', prNumber: 7, head, targetBase: base, runID: 12, runAttempt: 1, runURL: 'https://github.com/owner/repo/actions/runs/12' };
const stats = { files: 7, directories: 3, subsystems: 2, linesAdded: 412, linesRemoved: 88,
  symbolsAdded: 12, symbolsModified: 4, symbolsRemoved: 2, paths: ['src/a.ts', 'src/b.ts'] };
const history = { available: true,
  size: { available: true, sampleSize: 200, percentile: 82, metric: 500, classification: 'high' },
  fix: { files: 7, filesWithFixes: 3, weightedFixes: 14, considered: 5 } };
const ready = { version: 1, ...trusted, base, status: 'ready', elements: 2, connectors: 1, stats, history,
  markdown: '```mermaid\nflowchart LR\n  a["A"] --> b["B"]\n```\n' };
const pr = () => ({ number: 7, state: 'open', base: { sha: base, repo: { full_name: 'owner/repo' } }, head: { sha: head, ref: 'feature', repo: { id: 9 } } });
const association = () => ({ number: 7, head: { sha: head, repo: { id: 9 } } });
function apiFixture({ runChanges = {}, prChanges = {}, comments = [], associated = [association()] } = {}) {
  const calls = [];
  const run = { repository: { full_name: 'owner/repo' }, workflow_id: 3, event: 'pull_request', status: 'completed', head_sha: head,
    head_branch: 'feature', head_repository: { id: 9 }, pull_requests: associated, ...runChanges };
  return { calls, async request(method, path, body) {
    calls.push({ method, path, body });
    if (path.includes('/actions/runs/')) return run;
    if (path.includes('/actions/workflows/')) return { path: '.github/workflows/pr-diagram.yml' };
    if (path.endsWith('/pulls/7')) return { ...pr(), ...prChanges };
    if (method === 'POST' || method === 'PATCH') return { id: 10 };
    throw new Error(`Unexpected request: ${method} ${path}`);
  }, async pages(path) {
    if (path.endsWith('/comments')) return comments;
    if (path.endsWith('/pulls')) return [pr()];
    throw new Error(`Unexpected pages: ${path}`);
  } };
}

test('limits have documented defaults and reject invalid settings', () => {
  assert.deepEqual(limits({}), { elements: 80, connectors: 160 });
  assert.deepEqual(limits({ TLD_PR_MAX_ELEMENTS: '1', TLD_PR_MAX_CONNECTORS: '20' }), { elements: 1, connectors: 20 });
  for (const value of ['0', '-1', '', '1.5', 'word', '9007199254740992']) {
    assert.throws(() => limits({ TLD_PR_MAX_ELEMENTS: value }));
    assert.throws(() => limits({ TLD_PR_MAX_CONNECTORS: value }));
  }
});

test('valid results and all advisory statuses are accepted', () => {
  assert.equal(validateResult(JSON.stringify(ready), trusted).status, 'ready');
  for (const status of ['skipped', 'empty', 'error']) assert.equal(validateResult(JSON.stringify({ ...ready, status, markdown: '' }), trusted).status, status);
});

test('artifact identity, counts, stats, history, fenced content, and size are checked', () => {
  for (const changes of [{ prNumber: 8 }, { repository: 'attacker/repo' }, { head: base }, { targetBase: head }, { base: 'bad' },
    { elements: -1 }, { connectors: 1.2 }, { status: 'bad' }, { markdown: 'hello' },
    { stats: { ...stats, files: -1 } }, { stats: { ...stats, paths: [1] } },
    { history: { available: 'yes' } },
    { history: { available: true, size: { available: true, sampleSize: 200, percentile: 101, metric: 1, classification: 'high' } } },
    { history: { available: true, fix: { files: 1, filesWithFixes: 2, weightedFixes: -1 } } },
    { markdown: '```mermaid\nflowchart LR\n```\n@everyone\n```\n```' },
    { markdown: '```mermaid\nflowchart LR\n%%{init: {}}%%\n```' }]) {
    assert.throws(() => validateResult(JSON.stringify({ ...ready, ...changes }), trusted));
  }
  assert.throws(() => validateResult('x'.repeat(300_000), trusted));
  assert.throws(() => validateResult('not-json', trusted));
});

test('comments lead with change-risk stats, diagram, and run link instead of counts', () => {
  const body = commentBody(ready, trusted);
  for (const value of [marker, head.slice(0, 12), base.slice(0, 12), 'Change risk', 'Elevated', '82nd percentile',
    '+412 / −88', '3 of 7 files', 'Change stats', '```mermaid', trusted.runURL]) assert.ok(body.includes(value), value);
  assert.ok(!body.includes('elements'));
});

test('oversized bodies and generation errors have bounded fixed notices', () => {
  const oversized = commentBody({ ...ready, markdown: 'x'.repeat(90_000) }, trusted);
  assert.ok(oversized.includes('Diagram skipped'));
  assert.ok(oversized.length < 1000);
  for (const status of ['empty', 'error', 'skipped']) assert.ok(!commentBody({ ...ready, status, reason: '@everyone' }, trusted).includes('@everyone'));
});

test('publisher accepts associated same-repository and fork PRs', async () => {
  assert.deepEqual(await publicationContext(apiFixture(), 'owner/repo', 12, 'pr-diagram.yml'), trusted);
  // Fork ownership is established by GitHub's PR association, not the artifact.
  const api = apiFixture({ runChanges: { head_repository: { id: 123 } } });
  assert.equal((await publicationContext(api, 'owner/repo', 12, 'pr-diagram.yml')).prNumber, 7);
});

test('fork runs missing PR associations use verified commit associations', async () => {
  assert.equal((await publicationContext(apiFixture({ associated: [] }), 'owner/repo', 12, 'pr-diagram.yml')).prNumber, 7);
  assert.equal(await publicationContext(apiFixture({ associated: [], runChanges: { head_repository: { id: 123 } } }), 'owner/repo', 12, 'pr-diagram.yml'), null);
});

test('publisher ignores stale, closed, unrelated, and ambiguous runs', async () => {
  for (const options of [{ runChanges: { event: 'push' } }, { runChanges: { status: 'in_progress' } },
    { runChanges: { repository: { full_name: 'other/repo' } } }, { runChanges: { head_sha: base } },
    { prChanges: { state: 'closed' } }, { associated: [association(), association()] }]) {
    assert.equal(await publicationContext(apiFixture(options), 'owner/repo', 12, 'pr-diagram.yml'), null);
  }
  assert.equal(await publicationContext(apiFixture(), 'owner/repo', 12, 'other.yml'), null);
});

test('create and update exactly the bot-owned marked comment', async () => {
  const spoof = { id: 5, user: { type: 'User', login: 'someone' }, body: marker };
  const create = apiFixture({ comments: [spoof] });
  assert.equal(await upsertComment(create, trusted, ready), true);
  assert.equal(create.calls.at(-1).method, 'POST');
  const update = apiFixture({ comments: [spoof, { id: 8, user: { type: 'Bot', login: 'github-actions[bot]' }, body: marker }] });
  assert.equal(await upsertComment(update, trusted, { ...ready, status: 'skipped' }), true);
  assert.equal(update.calls.at(-1).path, '/repos/owner/repo/issues/comments/8');
  assert.equal(update.calls.at(-1).method, 'PATCH');
});

test('publication rechecks current revisions and does not regress newer comments', async () => {
  const stale = apiFixture({ prChanges: { base: { sha: head } } });
  assert.equal(await upsertComment(stale, trusted, ready), false);
  assert.ok(stale.calls.every(call => call.method === 'GET'));
  const newer = apiFixture({ comments: [{ user: { type: 'Bot', login: 'github-actions[bot]' }, body: `${marker}\n<!-- tld-run:13:1 -->` }] });
  assert.equal(await upsertComment(newer, trusted, ready), false);
  const same = apiFixture({ comments: [{ user: { type: 'Bot', login: 'github-actions[bot]' }, body: commentBody(ready, trusted) }] });
  assert.equal(await upsertComment(same, trusted, ready), true);
  assert.ok(same.calls.every(call => call.method === 'GET'));
});
