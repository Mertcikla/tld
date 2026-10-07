import { test } from 'node:test';
import assert from 'node:assert/strict';
import { record, isBugFix, classification, classificationLabel, ordinal, sizeStat, fixHistoryStat, parseChurnLog, parseFixLog } from './history.mjs';

test('bug-fix classifier matches the change-risk keywords and exclusions', () => {
  for (const subject of ['fix crash', 'Fix #12', 'closes #3', 'bug: handle nil', 'patch race', 'resolves the flake']) {
    assert.equal(isBugFix(subject), true, subject);
  }
  for (const subject of ['docs: fix typo', 'bump deps', 'chore: lint', 'style: format', 'add feature', '', null]) {
    assert.equal(isBugFix(subject), false, String(subject));
  }
});

test('classification is a tercile with human labels', () => {
  assert.equal(classification(0), 'low');
  assert.equal(classification(33), 'low');
  assert.equal(classification(34), 'moderate');
  assert.equal(classification(66), 'moderate');
  assert.equal(classification(67), 'high');
  assert.equal(classificationLabel('high'), 'Elevated');
  assert.equal(ordinal(82), '82nd');
  assert.equal(ordinal(93), '93rd');
  assert.equal(ordinal(11), '11th');
});

test('size rank uses the sample when it is large enough, else fixed bands', () => {
  const baseline = [0, 1, 2, 3, 4, 5, 6, 7];
  const high = sizeStat(7, baseline);
  assert.deepEqual(high, { available: true, sampleSize: 8, percentile: 100, metric: 7, classification: 'high' });
  assert.equal(sizeStat(4, baseline).percentile, 63);
  assert.equal(sizeStat(0, baseline).classification, 'low');
  const shallow = sizeStat(50, [1, 2]);
  assert.equal(shallow.available, false);
  assert.equal(shallow.classification, 'low');
  assert.equal(sizeStat(600, []).classification, 'high');
});

test('fix history sums recency-weighted bug fixes over the changed files', () => {
  const year = 365.25 * 86_400;
  const commits = [
    { timestamp: 1_700_000_000, subject: 'fix: crash', paths: ['src/a.go', 'src/b.go'] },
    { timestamp: 1_700_000_000, subject: 'docs: notes', paths: ['src/a.go'] },
    { timestamp: 1_700_000_000 - year, subject: 'bug: regression', paths: ['src/a.go'] },
    { timestamp: 1_700_000_000, subject: 'fix: unrelated', paths: ['src/z.go'] },
  ];
  const stat = fixHistoryStat(['src/a.go', 'src/b.go', 'src/c.go'], commits, 1_700_000_000 + year);
  assert.equal(stat.files, 3);
  assert.equal(stat.filesWithFixes, 2);
  assert.equal(stat.weightedFixes, 1.3);
  assert.equal(stat.considered, 3);
});

test('parsers read the git formats the generator emits', () => {
  const churn = parseChurnLog(`${record}abc\n10\t2\tfile.go\n\n${record}def\n-\t-\tbin.dat\n`);
  assert.deepEqual(churn, [12, 0]);
  const log = parseFixLog(`${record}1700000000\tfix: crash\nsrc/a.go\nsrc/b.go\n${record}1700000100\tdocs: notes\nsrc/c.go\n`);
  assert.deepEqual(log, [
    { timestamp: 1_700_000_000, subject: 'fix: crash', paths: ['src/a.go', 'src/b.go'] },
    { timestamp: 1_700_000_100, subject: 'docs: notes', paths: ['src/c.go'] },
  ]);
});
