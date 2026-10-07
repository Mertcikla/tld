import { createHash } from 'node:crypto';
import { classificationLabel, ordinal } from './history.mjs';

export const marker = '<!-- tld-pr-diagram:v1 -->';
export const maxResultBytes = 256 * 1024;
export const maxCommentBytes = 60 * 1024;
const shaPattern = /^[0-9a-f]{40}$/;
const repoPattern = /^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/;

export function limits(env) {
  const positive = (name, fallback) => {
    const value = env[name] ?? String(fallback);
    if (!/^[1-9][0-9]*$/.test(value) || !Number.isSafeInteger(Number(value))) {
      throw new Error(`${name} must be a positive integer`);
    }
    return Number(value);
  };
  return { elements: positive('TLD_PR_MAX_ELEMENTS', 80), connectors: positive('TLD_PR_MAX_CONNECTORS', 160) };
}

export function digest(value) {
  return createHash('sha256').update(value).digest('hex');
}

const statFields = ['files', 'directories', 'subsystems', 'linesAdded', 'linesRemoved', 'symbolsAdded', 'symbolsModified', 'symbolsRemoved'];
const bands = ['low', 'moderate', 'high'];

function validStats(stats) {
  if (stats === undefined) return;
  if (typeof stats !== 'object' || stats === null) throw new Error('Invalid change stats');
  for (const field of statFields) {
    if (!Number.isSafeInteger(stats[field]) || stats[field] < 0) throw new Error('Invalid change stats');
  }
  if (stats.paths !== undefined && (!Array.isArray(stats.paths) || stats.paths.length > 1000 ||
      stats.paths.some(value => typeof value !== 'string' || value.length > 1024))) throw new Error('Invalid change stats');
}

function validHistory(history) {
  if (history === undefined) return;
  if (typeof history !== 'object' || history === null || typeof history.available !== 'boolean') throw new Error('Invalid change history');
  const size = history.size;
  if (size !== undefined && size !== null) {
    if (typeof size !== 'object' || typeof size.available !== 'boolean' || !bands.includes(size.classification) ||
        !Number.isSafeInteger(size.sampleSize) || size.sampleSize < 0 || !Number.isSafeInteger(size.metric) || size.metric < 0 ||
        (size.percentile !== null && (!Number.isSafeInteger(size.percentile) || size.percentile < 0 || size.percentile > 100))) {
      throw new Error('Invalid change history');
    }
  }
  const fix = history.fix;
  if (fix !== undefined && fix !== null) {
    for (const field of ['files', 'filesWithFixes']) {
      if (!Number.isSafeInteger(fix[field]) || fix[field] < 0 || fix[field] > 1_000_000) throw new Error('Invalid change history');
    }
    if (typeof fix.weightedFixes !== 'number' || !Number.isFinite(fix.weightedFixes) || fix.weightedFixes < 0) throw new Error('Invalid change history');
  }
}

export function validateResult(raw, trusted) {
  if (Buffer.byteLength(raw) > maxResultBytes) throw new Error('Result artifact is too large');
  const result = JSON.parse(raw);
  if (result.version !== 1 || result.repository !== trusted.repository || result.prNumber !== trusted.prNumber ||
      result.head !== trusted.head || result.targetBase !== trusted.targetBase || !shaPattern.test(result.base) ||
      !['ready', 'skipped', 'empty', 'error'].includes(result.status)) throw new Error('Invalid result identity or status');
  for (const field of ['elements', 'connectors']) {
    if (!Number.isSafeInteger(result[field]) || result[field] < 0) throw new Error('Invalid diagram counts');
  }
  validStats(result.stats);
  validHistory(result.history);
  if (result.status === 'ready') {
    if (result.elements === 0 || typeof result.markdown !== 'string' ||
        !/^```mermaid\r?\nflowchart LR\r?\n[\s\S]*\r?\n```\s*$/.test(result.markdown) ||
        result.markdown.slice(10, result.markdown.lastIndexOf('```')).includes('```') ||
        result.markdown.includes('%%{') || result.markdown.includes('<script')) throw new Error('Invalid Mermaid output');
  }
  return result;
}

// statsSection renders the change-risk summary. It replaces bare element and
// connector counts with the signals a reviewer can act on: the change's size
// rank among recent commits, and whether its files have broken before. Detail
// stays collapsed so the comment stays short.
function statsSection(result) {
  const stats = result.stats ?? {};
  const history = result.history ?? {};
  const rows = [];
  const size = history.size;
  if (size) {
    const rank = size.available
      ? `${ordinal(size.percentile)} percentile of ${size.sampleSize} recent commits`
      : `too few recent commits to rank`;
    rows.push(['Size rank', `${classificationLabel(size.classification)} · ${rank}`]);
  }
  rows.push(['Lines changed', `+${stats.linesAdded ?? 0} / −${stats.linesRemoved ?? 0}`]);
  rows.push(['Files', `${stats.files ?? 0} across ${stats.directories ?? 0} ${(stats.directories ?? 0) === 1 ? 'directory' : 'directories'}`]);
  rows.push(['Symbols', `+${stats.symbolsAdded ?? 0} / ~${stats.symbolsModified ?? 0} / −${stats.symbolsRemoved ?? 0}`]);
  const fix = history.fix;
  if (fix) rows.push(['Prior bug fixes', fix.filesWithFixes === 0
    ? 'none in the files it touches'
    : `${fix.filesWithFixes} of ${fix.files} files (~${fix.weightedFixes} recency-weighted)`]);

  const headline = [];
  if (size) headline.push(classificationLabel(size.classification));
  headline.push(`+${stats.linesAdded ?? 0}/−${stats.linesRemoved ?? 0}`);
  headline.push(`${stats.files ?? 0} ${(stats.files ?? 0) === 1 ? 'file' : 'files'}`);
  if (fix && fix.filesWithFixes > 0) headline.push(`${fix.filesWithFixes} with prior fixes`);

  const table = ['| Metric | Value |', '|---|---|', ...rows.map(([key, value]) => `| ${key} | ${value} |`)].join('\n');
  return `**Change risk**: ${headline.join(' · ')}\n\n<details><summary>Change stats</summary>\n\n${table}\n\n</details>`;
}

export function commentBody(result, trusted) {
  const header = `${marker}\n<!-- tld-run:${trusted.runID}:${trusted.runAttempt ?? 1} -->\n### tld PR diagram\n\n`;
  const link = `[Workflow run](${trusted.runURL})`;
  const revisions = `\`${result.base.slice(0, 12)}\` → \`${result.head.slice(0, 12)}\`.`;
  const stats = statsSection(result);
  let content;
  if (result.status === 'ready') content = [revisions, stats, result.markdown.trim(), link].filter(Boolean).join('\n\n');
  else if (result.status === 'skipped') {
    const reason = { 'artifact-size': 'output exceeds the artifact size budget', 'comment-size': 'output exceeds the comment size budget' }[result.skipReason] ?? 'diagram exceeds the configured element or connector limit';
    content = [revisions, stats, `Diagram skipped: ${reason}.`, link].filter(Boolean).join('\n\n');
  }
  else if (result.status === 'empty') content = [revisions, `No indexable changes to diagram.`, link].filter(Boolean).join('\n\n');
  else content = [`Diagram generation was unsuccessful. See the workflow logs for details.`, link].join('\n\n');
  if (Buffer.byteLength(header + content) > maxCommentBytes) {
    return commentBody({ ...result, status: 'skipped', skipReason: 'comment-size' }, trusted);
  }
  return header + content;
}

export class GitHub {
  constructor(token, apiURL = 'https://api.github.com') {
    this.token = token;
    this.apiURL = apiURL.replace(/\/$/, '');
  }
  async request(method, path, body) {
    const response = await fetch(`${this.apiURL}${path}`, {
      method,
      headers: { Authorization: `Bearer ${this.token}`, Accept: 'application/vnd.github+json', 'X-GitHub-Api-Version': '2022-11-28', 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(30_000),
    });
    if (!response.ok) throw new Error(`GitHub ${method} ${path}: HTTP ${response.status}`);
    return response.json();
  }
  async pages(path) {
    const items = [];
    for (let page = 1; ; page++) {
      const batch = await this.request('GET', `${path}${path.includes('?') ? '&' : '?'}per_page=100&page=${page}`);
      items.push(...batch);
      if (batch.length < 100) return items;
    }
  }
}

// The PR identity comes from GitHub, never from the downloaded artifact.
export async function publicationContext(api, repository, runID, sourceWorkflow, serverURL = 'https://github.com') {
  if (!repoPattern.test(repository) || !/^[1-9][0-9]*$/.test(String(runID))) throw new Error('Invalid workflow identity');
  const prefix = `/repos/${repository}`;
  const run = await api.request('GET', `${prefix}/actions/runs/${runID}`);
  const workflow = await api.request('GET', `${prefix}/actions/workflows/${run.workflow_id}`);
  if (run.repository.full_name !== repository || run.event !== 'pull_request' || run.status !== 'completed' ||
      workflow.path !== `.github/workflows/${sourceWorkflow}` || !shaPattern.test(run.head_sha)) return null;
  let associated = run.pull_requests ?? [];
  if (associated.length === 0) {
    associated = await api.pages(`${prefix}/commits/${run.head_sha}/pulls`);
    associated = associated.filter(pr => pr.head.sha === run.head_sha && pr.head.ref === run.head_branch &&
      pr.head.repo?.id === run.head_repository?.id);
  }
  if (associated.length !== 1) return null;
  const pr = await api.request('GET', `${prefix}/pulls/${associated[0].number}`);
  if (pr.state !== 'open' || pr.base.repo.full_name !== repository || pr.head.sha !== run.head_sha ||
      associated[0].head?.sha !== pr.head.sha || associated[0].head?.repo?.id !== pr.head.repo?.id) return null;
  return { repository, prNumber: pr.number, head: pr.head.sha, targetBase: pr.base.sha,
    runID: Number(runID), runAttempt: run.run_attempt ?? 1, runURL: `${serverURL}/${repository}/actions/runs/${runID}` };
}

export async function upsertComment(api, trusted, result) {
  // Recheck immediately before a mutation; a newer PR update may have arrived.
  const prefix = `/repos/${trusted.repository}`;
  const pr = await api.request('GET', `${prefix}/pulls/${trusted.prNumber}`);
  if (pr.state !== 'open' || pr.head.sha !== trusted.head || pr.base.sha !== trusted.targetBase) return false;
  const comments = await api.pages(`${prefix}/issues/${trusted.prNumber}/comments`);
  const existing = comments.find(comment => comment.user?.type === 'Bot' &&
    comment.user?.login === 'github-actions[bot]' && comment.body?.startsWith(marker));
  const previous = existing?.body?.match(/<!-- tld-run:([0-9]+):([0-9]+) -->/);
  if (previous && (Number(previous[1]) > trusted.runID ||
      (Number(previous[1]) === trusted.runID && Number(previous[2]) > (trusted.runAttempt ?? 1)))) return false;
  const body = commentBody(result, trusted);
  if (existing) {
    if (existing.body !== body) await api.request('PATCH', `${prefix}/issues/comments/${existing.id}`, { body });
  } else await api.request('POST', `${prefix}/issues/${trusted.prNumber}/comments`, { body });
  return true;
}
