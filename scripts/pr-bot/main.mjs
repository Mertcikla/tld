import { readFileSync, writeFileSync, appendFileSync, mkdirSync, readdirSync, statSync, existsSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { limits, digest, GitHub, publicationContext, validateResult, upsertComment, maxResultBytes } from './lib.mjs';

const env = process.env;
const tldVersion = 'v2.4.0-beta.4';
const mode = process.argv[2];
const root = resolve(fileURLToPath(new URL('../..', import.meta.url)));
const stateDir = join(env.RUNNER_TEMP ?? '/tmp', 'tld-pr-bot');
const resultDir = join(stateDir, 'result');
const dataDir = join(stateDir, 'data');
const configDir = join(stateDir, 'config');
const binaryDir = join(stateDir, 'bin');
mkdirSync(resultDir, { recursive: true });
const event = JSON.parse(readFileSync(env.GITHUB_EVENT_PATH, 'utf8'));
const output = (key, value) => { if (env.GITHUB_OUTPUT) appendFileSync(env.GITHUB_OUTPUT, `${key}<<TLD_OUTPUT\n${value}\nTLD_OUTPUT\n`); };
const summary = text => { console.log(text); if (env.GITHUB_STEP_SUMMARY) appendFileSync(env.GITHUB_STEP_SUMMARY, `${text}\n`); };
const execute = (command, args, cwd, options = {}) => {
  const result = spawnSync(command, args, { cwd, encoding: 'utf8', timeout: 25 * 60_000, maxBuffer: 8 * 1024 * 1024, ...options });
  if (result.status !== 0) throw new Error(`${command} failed: ${result.error?.message ?? result.stderr?.slice(-6000) ?? result.status}`);
  return result.stdout?.trim() ?? '';
};
const repository = resolve(env.INPUT_REPOSITORY_PATH || env.GITHUB_WORKSPACE);
const statePath = join(stateDir, 'state.json');
const readState = () => JSON.parse(readFileSync(statePath, 'utf8'));
const saveResult = result => writeFileSync(join(resultDir, 'result.json'), JSON.stringify(result));

function sourceDigest(directory) {
  const inputs = [];
  const excluded = new Set(['.git', 'node_modules', 'dist', 'tmp', '.bin', 'data', 'public', 'coverage', 'playwright-report', 'test-results']);
  const visit = (dir, prefix = '') => {
    for (const entry of readdirSync(dir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
      if (entry.isSymbolicLink() || excluded.has(entry.name)) continue;
      const name = `${prefix}${entry.name}`;
      if (entry.isDirectory()) visit(join(dir, entry.name), `${name}/`);
      else if (/\.(go|sql|json|mjs|ts|tsx|html|yml|yaml|css|scss)$/.test(name) || ['go.mod', 'go.sum', 'build-assets/icons.tar.gz'].includes(name)) {
        inputs.push(name, digest(readFileSync(join(dir, entry.name))));
      }
    }
  };
  visit(directory);
  return digest(inputs.join('\n'));
}

function initialize() {
  for (const dir of [dataDir, configDir, binaryDir]) mkdirSync(dir, { recursive: true });
  const pr = event.pull_request;
  if (env.INPUT_MODE !== 'generate' || !pr || event.repository.full_name !== env.GITHUB_REPOSITORY) {
    throw new Error('Generation requires generate mode and a pull_request event');
  }
  const head = pr.head.sha;
  const targetBase = pr.base.sha;
  for (const sha of [targetBase, head]) {
    if (!/^[0-9a-f]{40}$/.test(sha)) throw new Error('Invalid revision');
    try { execute('git', ['cat-file', '-e', `${sha}^{commit}`], repository); }
    catch { execute('git', ['fetch', '--no-tags', 'origin', sha], repository); }
  }
  const base = execute('git', ['merge-base', targetBase, head], repository);
  const state = { version: 1, repository: env.GITHUB_REPOSITORY, prNumber: pr.number,
    targetBase, base, head, status: 'error', elements: 0, connectors: 0 };
  writeFileSync(statePath, JSON.stringify(state));
  saveResult(state); // Remains a valid advisory failure artifact if setup fails.
  const source = sourceDigest(root);
  const tools = `tld${tldVersion.slice(1)}-go1.26.2-node24-scipgo0.2.7-scipts0.4.0`;
  const namespace = `tld-pr-v1-${env.RUNNER_OS}-${env.RUNNER_ARCH}-${digest(`${env.GITHUB_REPOSITORY}|${source}|${tools}|${env.INPUT_INDEXERS}|${env.INPUT_PREPARE_COMMAND}`).slice(0, 32)}`;
  const scope = `pr-${state.prNumber}`;
  output('binary-key', `tld-binary-v2-${env.RUNNER_OS}-${env.RUNNER_ARCH}-${tools}`);
  output('tld-version', tldVersion);
  output('binary-dir', binaryDir);
  output('data-dir', dataDir);
  output('result-dir', resultDir);
  output('cache-key', `${namespace}-${scope}-${base}-${head}-${env.GITHUB_RUN_ID}-${env.GITHUB_RUN_ATTEMPT}`);
  output('restore-keys', [`${namespace}-${scope}-${base}-`, `${namespace}-${scope}-`].join('\n'));
  const enabled = (env.INPUT_INDEXERS || 'go typescript').split(/[\s,]+/).filter(Boolean);
  if (enabled.some(value => !['go', 'typescript'].includes(value))) throw new Error('Supported indexers: go, typescript');
  output('go-indexer', enabled.includes('go'));
  output('typescript-indexer', enabled.includes('typescript'));
  limits(env);
}

function generate() {
  const state = readState();
  if (env.INPUT_TOOLS_READY === 'false') throw new Error('tld or SCIP installation failed');
  const cap = limits(env);
  const depth = env.INPUT_CONTEXT_DEPTH || '0';
  if (!/^[0-9]+$/.test(depth) || Number(depth) > 100) throw new Error('context-depth must be an integer from 0 to 100');
  const enabled = (env.INPUT_INDEXERS || 'go typescript').split(/[\s,]+/);
  for (const tool of ['go', 'typescript']) {
    if (enabled.includes(tool)) {
      const version = execute(`scip-${tool}`, ['--version'], repository);
      const expected = tool === 'go' ? /(^|[^0-9])0\.2\.7([^0-9]|$)/ : /(^|[^0-9])0\.4\.0([^0-9]|$)/;
      if (!expected.test(version)) throw new Error(`Unexpected scip-${tool} version: ${version}`);
    }
  }
  const reportPath = join(stateDir, 'compare-report.json');
  const result = spawnSync(join(binaryDir, 'tld'), ['git', 'compare', repository, state.base, state.head,
    '--markdown', '--depth', depth, '--max-nodes', '0', '--max-bytes', '0',
    '--max-elements', String(cap.elements), '--max-connectors', String(cap.connectors),
    '--report-json', reportPath, '--prepare-command', env.INPUT_PREPARE_COMMAND || '', '--data-dir', dataDir], {
    cwd: repository, encoding: 'utf8', timeout: 25 * 60_000, maxBuffer: 8 * 1024 * 1024,
    env: { ...env, TLD_CONFIG_DIR: configDir, TLD_UPDATES_AUTO: 'false', GOWORK: 'off' },
  });
  if (result.stderr) console.log(result.stderr);
  if (result.status !== 0) throw new Error(result.error?.message ?? `tld exited ${result.status}`);
  const report = JSON.parse(readFileSync(reportPath, 'utf8'));
  if (report.base !== state.base || report.head !== state.head || !Array.isArray(report.warnings)) throw new Error('Invalid compare report');
  const payload = { ...state, ...report, repository: state.repository, prNumber: state.prNumber, targetBase: state.targetBase,
    skipReason: report.status === 'skipped' ? 'diagram-limit' : undefined,
    markdown: report.status === 'ready' ? result.stdout : '' };
  if (report.warnings.length) {
    console.log(report.warnings.join('\n'));
    payload.status = 'error';
    payload.markdown = '';
    payload.reason = 'Indexing produced warnings; diagram omitted.';
  }
  if (Buffer.byteLength(JSON.stringify(payload)) > maxResultBytes) {
    payload.status = 'skipped';
    payload.markdown = '';
    payload.warnings = [];
    payload.reason = 'Output exceeds the artifact size budget.';
    payload.skipReason = 'artifact-size';
  }
  saveResult(payload);
  output('status', payload.status);
  output('cache-ready', report.warnings.length === 0);
  summary(`tld PR diagram: ${payload.status} (${report.elements} elements, ${report.connectors} connectors). ${payload.reason ?? ''}`);
}

async function checkPublish() {
  const runID = env.INPUT_RUN_ID || event.workflow_run?.id;
  // Never accept an arbitrary run selected through artifact contents.
  if (String(runID) !== String(event.workflow_run?.id)) throw new Error('run-id does not match the workflow_run event');
  const api = new GitHub(env.GITHUB_TOKEN, env.GITHUB_API_URL);
  const trusted = await publicationContext(api, env.GITHUB_REPOSITORY, runID,
    env.INPUT_SOURCE_WORKFLOW || 'pr-diagram.yml', env.GITHUB_SERVER_URL);
  output('publish', Boolean(trusted));
  if (trusted) {
    writeFileSync(join(stateDir, 'publication.json'), JSON.stringify(trusted));
    output('run-id', trusted.runID);
    output('pr-number', trusted.prNumber);
  } else summary('tld PR diagram: ignored stale, closed, or unrelated workflow run.');
}

async function publish() {
  const trusted = JSON.parse(readFileSync(join(stateDir, 'publication.json'), 'utf8'));
  const path = join(stateDir, 'download', 'result.json');
  let result;
  try {
    if (statSync(path).size > maxResultBytes) throw new Error('Result artifact is too large');
    result = validateResult(readFileSync(path, 'utf8'), trusted);
  } catch (error) {
    summary(`tld PR diagram artifact rejected: ${error.message}`);
    // Malformed/unavailable artifacts can only produce a fixed failure notice.
    result = { ...trusted, base: trusted.targetBase, status: 'error', elements: 0, connectors: 0 };
  }
  await upsertComment(new GitHub(env.GITHUB_TOKEN, env.GITHUB_API_URL), trusted, result);
}

try {
  if (mode === 'init') initialize();
  else if (mode === 'generate') generate();
  else if (mode === 'publish-check') await checkPublish();
  else if (mode === 'publish') await publish();
  else throw new Error(`Unsupported mode: ${mode}`);
} catch (error) {
  output('status', 'error');
  output('cache-ready', 'false');
  if (['init', 'generate'].includes(mode) && existsSync(statePath)) {
    saveResult({ ...readState(), status: 'error', reason: 'Diagram generation was unsuccessful; see workflow logs.' });
  }
  summary(`tld PR diagram advisory error: ${error.message}`);
}
