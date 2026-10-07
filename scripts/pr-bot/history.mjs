// Change-history statistics for the PR comment.
//
// The generator already knows the change's shape (files, lines, symbols) from
// the compare report. These helpers add the two history-derived signals copied
// from repowise's change-risk layer: where the change's size ranks among recent
// commits, and whether the files it touches have broken before. Everything is
// derived from git; the bot needs no index, API key, or network for it.
//
// Counts are shipped now; the fix-history percentile is deliberately deferred.

// Record separator used to delimit per-commit blocks in git --format output.
// It is not whitespace, so execute()'s trim cannot eat a leading marker.
export const record = '\x1e';

// A change younger than this many sampled commits cannot be ranked.
export const minBaseline = 8;

// Absolute churn bands used when the repository has too little history to rank
// against, mirroring the doc's fallback band.
export const fallbackChurn = [100, 500];

const included = /\b(fix|fixes|fixed|bug|patch|resolves|resolved|closes|closed)\b/i;
const excluded = /\b(docs|typo|bump|deps|chore|lint|format|style)\b/i;

// isBugFix mirrors the change-risk classifier: a keyword match that is not a
// documentation, dependency, or formatting commit.
export function isBugFix(subject) {
  return typeof subject === 'string' && included.test(subject) && !excluded.test(subject);
}

export function classification(percentile) {
  if (percentile < 34) return 'low';
  if (percentile < 67) return 'moderate';
  return 'high';
}

const labels = { low: 'Below typical', moderate: 'Typical', high: 'Elevated' };

export function classificationLabel(value) {
  return labels[value] ?? labels.moderate;
}

// ordinal renders a whole number as 1st/2nd/93rd/11th for the visible percentile.
export function ordinal(value) {
  const suffix = value % 100 >= 11 && value % 100 <= 13 ? 'th' : { 1: 'st', 2: 'nd', 3: 'rd' }[value % 10] ?? 'th';
  return `${value}${suffix}`;
}

// sizeStat ranks a change's total churn against recent commits. Without at
// least minBaseline samples it falls back to fixed bands and reports
// available: false.
export function sizeStat(changeChurn, baseline) {
  const churn = Math.max(0, changeChurn | 0);
  if (!Array.isArray(baseline) || baseline.length < minBaseline) {
    const band = churn <= fallbackChurn[0] ? 'low' : churn <= fallbackChurn[1] ? 'moderate' : 'high';
    return { available: false, sampleSize: Array.isArray(baseline) ? baseline.length : 0, percentile: null, metric: churn, classification: band };
  }
  const below = baseline.filter(value => value <= churn).length;
  const percentile = Math.round((100 * below) / baseline.length);
  return { available: true, sampleSize: baseline.length, percentile, metric: churn, classification: classification(percentile) };
}

// fixHistoryStat sums recency-weighted bug fixes over the changed files. A fix
// from a year before the change counts a half, two years a quarter, exactly as
// the change-risk layer weights them. Summary only: the caller names counts,
// not files.
export function fixHistoryStat(paths, commits, asOf) {
  const target = new Set(paths);
  const weights = new Map();
  let considered = 0;
  for (const commit of commits) {
    if (!isBugFix(commit.subject)) continue;
    considered++;
    const ageYears = Math.max(0, asOf - commit.timestamp) / (365.25 * 86_400);
    const weight = 0.5 ** ageYears;
    for (const file of commit.paths) {
      if (!target.has(file)) continue;
      weights.set(file, (weights.get(file) ?? 0) + weight);
    }
  }
  const filesWithFixes = [...weights.values()].filter(value => value > 0).length;
  const weightedFixes = Math.round([...weights.values()].reduce((sum, value) => sum + value, 0) * 10) / 10;
  return { files: target.size, filesWithFixes, weightedFixes, considered };
}

// parseChurnLog reads `git log --format=RECORD%H --numstat` into one total
// churn (added + removed) per commit. Binary files report "-", counted as zero.
export function parseChurnLog(raw) {
  const churns = [];
  let total = null;
  for (const line of raw.split('\n')) {
    if (line.startsWith(record)) {
      if (total !== null) churns.push(total);
      total = 0;
      continue;
    }
    if (total === null) continue;
    const fields = line.split('\t');
    if (fields.length < 3) continue;
    total += Number(fields[0]) || 0;
    total += Number(fields[1]) || 0;
  }
  if (total !== null) churns.push(total);
  return churns;
}

// parseFixLog reads `git log --format=RECORD%ct%x09%s --name-only` into commit
// records with the timestamp, subject, and files it touched.
export function parseFixLog(raw) {
  const commits = [];
  let current = null;
  for (const line of raw.split('\n')) {
    if (line.startsWith(record)) {
      const [timestamp, subject = ''] = line.slice(record.length).split('\t');
      current = { timestamp: Number(timestamp) || 0, subject, paths: [] };
      commits.push(current);
      continue;
    }
    if (!current) continue;
    const file = line.trim();
    if (file) current.paths.push(file);
  }
  return commits;
}
