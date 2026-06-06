export interface CollaborationIdentity {
  client_id: string
  user_id: string
  username: string
}

const storageKey = 'tld.collaboration.identity.v1'

function makeID(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return `client-${Math.random().toString(36).slice(2)}-${Date.now().toString(36)}`
}

function sanitizeUserID(value: string): string {
  const cleaned = value.trim().toLowerCase().replace(/[^a-z0-9._-]+/g, '-').replace(/^[._-]+|[._-]+$/g, '')
  return cleaned.slice(0, 64).replace(/^[._-]+|[._-]+$/g, '')
}

function readStored(): Partial<CollaborationIdentity> {
  if (typeof localStorage === 'undefined') return {}
  try {
    const parsed = JSON.parse(localStorage.getItem(storageKey) || '{}') as Partial<CollaborationIdentity>
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

function writeStored(identity: CollaborationIdentity) {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(storageKey, JSON.stringify(identity))
}

export function getCollaborationIdentity(): CollaborationIdentity {
  const stored = readStored()
  const clientID = typeof stored.client_id === 'string' && stored.client_id.trim() ? stored.client_id.trim() : makeID()
  const fallback = `user${Math.max(1, parseInt(clientID.replace(/\D/g, '').slice(-2), 10) || 1)}`
  const userID = sanitizeUserID(typeof stored.user_id === 'string' ? stored.user_id : '') || fallback
  const username = typeof stored.username === 'string' && stored.username.trim() ? stored.username.trim().slice(0, 80) : userID
  const identity = { client_id: clientID, user_id: userID, username }
  writeStored(identity)
  return identity
}

export function saveCollaborationIdentity(input: Pick<CollaborationIdentity, 'user_id' | 'username'>): CollaborationIdentity {
  const current = getCollaborationIdentity()
  const userID = sanitizeUserID(input.user_id)
  const username = input.username.trim()
  if (!userID) throw new Error('User ID is required')
  if (!username) throw new Error('Username is required')
  if (username.length > 80) throw new Error('Username must be 80 characters or fewer')
  const identity = { client_id: current.client_id, user_id: userID, username }
  writeStored(identity)
  return identity
}

export function collaborationIdentityHeaders(): Record<string, string> {
  const identity = getCollaborationIdentity()
  return {
    'x-tld-collab-client-id': identity.client_id,
    'x-tld-collab-user-id': identity.user_id,
    'x-tld-collab-username': identity.username,
  }
}
