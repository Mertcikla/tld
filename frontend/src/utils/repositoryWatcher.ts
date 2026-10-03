import type { RepositoryWatchStatus } from '../api/client'

type Activity = { label: string; detail: string; step: string; active: boolean }
export function watcherActivity(status: RepositoryWatchStatus | null): Activity {
  if (!status) return { label: 'Checking watcher…', detail: 'Loading the latest watcher status.', step: '', active: true }
  if (!status.running) return { label: status.error ? 'Watcher stopped after an error' : 'Stopped', detail: 'Start watching to keep the live map up to date.', step: '', active: false }
  if (status.stopRequested || status.state === 'stopping') return { label: 'Stopping…', detail: 'Finishing or cancelling the current work.', step: '', active: true }
  if (status.state === 'error') return { label: 'Retrying after an error', detail: status.error || 'The watcher will retry pending changes.', step: '', active: false }
  if (status.state === 'starting') return { label: 'Starting…', detail: 'Connecting to the repository and checking Git inputs.', step: 'listen', active: true }
  if (status.state === 'debouncing') return { label: 'Debouncing changes', detail: `Waiting ${status.debounceMs || 500} ms for edits to settle before indexing.`, step: 'listen', active: true }
  if (status.stage === 'waiting-indexer') return { label: 'Waiting for indexer', detail: 'Another indexing operation holds this repository. Pending changes will run when it finishes.', step: 'index', active: true }
  if (status.stage === 'waiting-embedding') return { label: 'Waiting for embedding service', detail: 'An embedding request is in flight.', step: 'embed', active: true }
  if (status.stage === 'embedding') return { label: 'Embedding symbols', detail: 'Preparing and saving embeddings for indexed code.', step: 'embed', active: true }
  if (status.stage === 'live-map' || status.stage === 'materialize') return { label: 'Updating live map', detail: 'Saving the diagram and change overlays.', step: 'map', active: true }
  if (status.state === 'scanning') {
    const stages: Record<string, string> = { discover: 'Discovering sources', 'tree-sitter': 'Parsing sources', scip: 'Indexing symbols', relationships: 'Resolving relationships', infra: 'Indexing infrastructure', verify: 'Verifying index', publish: 'Publishing snapshot' }
    return { label: stages[status.stage] || 'Indexing changes', detail: 'Scanning code and updating the repository index.', step: 'index', active: true }
  }
  return { label: 'Idle · watching for changes', detail: 'Listening for file edits and Git changes.', step: 'listen', active: false }
}

