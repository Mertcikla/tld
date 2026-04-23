const GITHUB_API_BASE = 'https://api.github.com'
const MIN_REQUEST_GAP_MS = 300

type RepoVisibility = 'public' | 'private' | 'unknown'

let queue: Promise<void> = Promise.resolve()
let lastRequestAt = 0
let rateLimitedUntil = 0

const visibilityInFlight = new Map<string, Promise<RepoVisibility>>()

function wait(ms: number): Promise<void> {
  if (ms <= 0) return Promise.resolve()
  return new Promise((resolve) => {
    window.setTimeout(resolve, ms)
  })
}

function toNumber(value: string | null): number | null {
  if (!value) return null
  const parsed = Number(value)
  return Number.isFinite(parsed) ? parsed : null
}

function getAuthHeader(): string | null {
  const clientId = import.meta.env.VITE_GH_CLIENT_ID?.trim()
  const clientSecret = import.meta.env.VITE_GH_CLIENT_SECRET?.trim()
  if (!clientId || !clientSecret) return null
  return `Basic ${window.btoa(`${clientId}:${clientSecret}`)}`
}

function updateRateLimitState(response: Response) {
  const now = Date.now()
  const retryAfterSeconds = toNumber(response.headers.get('retry-after'))
  const remaining = toNumber(response.headers.get('x-ratelimit-remaining'))
  const resetUnixSeconds = toNumber(response.headers.get('x-ratelimit-reset'))

  if (retryAfterSeconds !== null && retryAfterSeconds > 0) {
    rateLimitedUntil = Math.max(rateLimitedUntil, now + retryAfterSeconds * 1000)
  }

  if (remaining === 0 && resetUnixSeconds !== null) {
    // Add a small safety buffer after reset time to avoid immediate re-limit.
    rateLimitedUntil = Math.max(rateLimitedUntil, resetUnixSeconds * 1000 + 500)
  }

  if ((response.status === 403 || response.status === 429) && retryAfterSeconds === null && remaining === 0) {
    rateLimitedUntil = Math.max(rateLimitedUntil, now + 60_000)
  }
}

function enqueueRequest<T>(request: () => Promise<T>): Promise<T> {
  const run = async () => {
    const now = Date.now()
    if (rateLimitedUntil > now) {
      await wait(rateLimitedUntil - now)
    }

    const elapsedSinceLast = Date.now() - lastRequestAt
    if (elapsedSinceLast < MIN_REQUEST_GAP_MS) {
      await wait(MIN_REQUEST_GAP_MS - elapsedSinceLast)
    }

    lastRequestAt = Date.now()
    return request()
  }

  const chained = queue.then(run, run)
  queue = chained.then(() => undefined, () => undefined)
  return chained
}

export async function githubRequest(path: string): Promise<Response> {
  return enqueueRequest(async () => {
    const headers = new Headers({
      Accept: 'application/vnd.github+json',
    })
    const authHeader = getAuthHeader()
    if (authHeader) headers.set('Authorization', authHeader)

    let response = await fetch(`${GITHUB_API_BASE}${path}`, {
      method: 'GET',
      headers,
    })

    // If the request failed with 401 but we provided credentials, retry without credentials.
    // This handles cases where client ID/secret are misconfigured but the resource is public.
    if (response.status === 401 && authHeader) {
      headers.delete('Authorization')
      response = await fetch(`${GITHUB_API_BASE}${path}`, {
        method: 'GET',
        headers,
      })
    }

    updateRateLimitState(response)
    return response
  })
}

export function getGithubRepoVisibility(repoSlug: string): Promise<RepoVisibility> {
  const inFlight = visibilityInFlight.get(repoSlug)
  if (inFlight) return inFlight

  const promise = (async () => {
    const response = await githubRequest(`/repos/${repoSlug}`)
    if (response.status === 200) return 'public'
    if (response.status === 404 || response.status === 401) return 'private'
    if (response.status === 403 || response.status === 429) {
      const remaining = toNumber(response.headers.get('x-ratelimit-remaining'))
      if (remaining === 0 || Date.now() < rateLimitedUntil) return 'unknown'
      return 'private'
    }
    return 'unknown'
  })().finally(() => {
    visibilityInFlight.delete(repoSlug)
  })

  visibilityInFlight.set(repoSlug, promise)
  return promise
}
