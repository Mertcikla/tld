import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, chmodSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { spawnSync } from 'node:child_process';

const script = resolve('scripts/pr-bot/main.mjs');
function fixture(t) {
  const dir = mkdtempSync(join(tmpdir(), 'tld-action-test-'));
  t.after(() => rmSync(dir, { recursive: true, force: true }));
  const repo = join(dir, 'repo');
  mkdirSync(repo);
  const git = args => {
    const result = spawnSync('git', args, { cwd: repo, encoding: 'utf8' });
    assert.equal(result.status, 0, result.stderr);
    return result.stdout.trim();
  };
  git(['init', '-b', 'main']);
  git(['config', 'user.name', 'Test']);
  git(['config', 'user.email', 'test@example.com']);
  writeFileSync(join(repo, 'file.txt'), 'base');
  git(['add', '.']); git(['commit', '-m', 'base']);
  const base = git(['rev-parse', 'HEAD']);
  writeFileSync(join(repo, 'file.txt'), 'head');
  git(['add', '.']); git(['commit', '-m', 'head']);
  const head = git(['rev-parse', 'HEAD']);
  const eventPath = join(dir, 'event.json');
  writeFileSync(eventPath, JSON.stringify({ repository: { full_name: 'owner/repo' }, pull_request: { number: 7, base: { sha: base }, head: { sha: head } } }));
  const env = { ...process.env, RUNNER_TEMP: dir, RUNNER_OS: 'Linux', RUNNER_ARCH: 'X64',
    GITHUB_EVENT_PATH: eventPath, GITHUB_WORKSPACE: repo, GITHUB_REPOSITORY: 'owner/repo', GITHUB_RUN_ID: '12',
    GITHUB_RUN_ATTEMPT: '1', GITHUB_OUTPUT: join(dir, 'output'), GITHUB_STEP_SUMMARY: join(dir, 'summary'),
    INPUT_MODE: 'generate', INPUT_REPOSITORY_PATH: repo, INPUT_INDEXERS: 'go', INPUT_TOOLS_READY: 'true' };
  const run = (mode, override = {}) => {
    const result = spawnSync(process.execPath, [script, mode], { env: { ...env, ...override }, encoding: 'utf8' });
    assert.equal(result.status, 0, result.stderr);
    return result.stdout;
  };
  run('init');
  const bin = join(dir, 'tld-pr-bot', 'bin');
  const stub = (name, source) => {
    writeFileSync(join(bin, name), `#!${process.execPath}\n${source}\n`);
    chmodSync(join(bin, name), 0o700);
  };
  stub('scip-go', 'console.log("scip-go 0.2.7")');
  stub('tld', `
    const fs = require('fs');
    const args = process.argv.slice(2);
    const warnings = process.env.FAKE_WARNINGS ? ['indexer failed'] : [];
    if (process.env.FAKE_ERROR) process.exit(1);
    fs.writeFileSync(args[args.indexOf('--report-json') + 1], JSON.stringify({
      status: process.env.FAKE_STATUS || 'ready', base: args[3], head: args[4], elements: 2, connectors: 1, warnings
    }));
    console.log('\\x60\\x60\\x60mermaid\\nflowchart LR\\n  a["A"] --> b["B"]\\n\\x60\\x60\\x60');
  `);
  env.PATH = `${bin}:${env.PATH}`;
  const result = () => JSON.parse(readFileSync(join(dir, 'tld-pr-bot', 'result', 'result.json'), 'utf8'));
  return { run, result, env, dir };
}

test('producer initializes identities and creates a valid diagram artifact', t => {
  const f = fixture(t);
  f.run('generate');
  assert.equal(f.result().status, 'ready');
  assert.equal(f.result().prNumber, 7);
  assert.ok(f.result().markdown.includes('```mermaid'));
  assert.ok(readFileSync(f.env.GITHUB_OUTPUT, 'utf8').includes('cache-ready<<TLD_OUTPUT\ntrue'));
});

test('partial indexes omit diagrams and do not save the cache', t => {
  const f = fixture(t);
  f.run('generate', { FAKE_WARNINGS: '1' });
  assert.equal(f.result().status, 'error');
  assert.equal(f.result().markdown, '');
  assert.ok(readFileSync(f.env.GITHUB_OUTPUT, 'utf8').includes('cache-ready<<TLD_OUTPUT\nfalse'));
});

test('install and indexing failures preserve an advisory failure artifact', t => {
  const f = fixture(t);
  f.run('generate', { INPUT_TOOLS_READY: 'false' });
  assert.equal(f.result().status, 'error');
  f.run('generate', { FAKE_ERROR: '1' });
  assert.equal(f.result().status, 'error');
});

test('invalid limits are advisory and skipped renders still save healthy indexes', t => {
  const f = fixture(t);
  f.run('init', { TLD_PR_MAX_ELEMENTS: '0' });
  assert.equal(f.result().status, 'error');
  f.run('generate', { FAKE_STATUS: 'skipped' });
  assert.equal(f.result().status, 'skipped');
  assert.equal(f.result().markdown, '');
  assert.ok(readFileSync(f.env.GITHUB_OUTPUT, 'utf8').includes('cache-ready<<TLD_OUTPUT\ntrue'));
});

test('PR caches preserve the comparison and config changes namespace caches', t => {
  const f = fixture(t);
  assert.notEqual(f.result().base, f.result().head);
  assert.equal(f.result().prNumber, 7);
  const first = readFileSync(f.env.GITHUB_OUTPUT, 'utf8');
  const release = first.match(/tld-version<<TLD_OUTPUT\n(v[^\n]+)/)[1];
  assert.ok(first.includes(`tld-binary-v2-Linux-X64-tld${release.slice(1)}-`));
  f.run('init', { INPUT_PREPARE_COMMAND: 'echo setup' });
  const keys = readFileSync(f.env.GITHUB_OUTPUT, 'utf8').match(/cache-key<<TLD_OUTPUT\n([^\n]+)/g);
  assert.ok(first.includes('-pr-7-'));
  assert.notEqual(keys.at(-1), keys.at(-2));
});
