import type {
  LoginRequest,
  LoginResponse,
  User,
  ApiToken,
  ApiTokenCreated,
  SessionTemplate,
  SessionTemplateCreate,
  Session,
  SessionDetail,
  SessionCreateRequest,
  Client,
  ServerStatus,
  ApiMapping,
} from './types'
import { mappingToApi, apiToMapping } from './types'

class ApiClientError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.name = 'ApiClientError'
    this.status = status
  }
}

type OnUnauthorized = () => void

let onUnauthorizedCallback: OnUnauthorized | null = null

export function setOnUnauthorized(cb: OnUnauthorized) {
  onUnauthorizedCallback = cb
}

export function setAuthToken(token: string | null) {
  if (token) {
    sessionStorage.setItem('ct-token', token)
  } else {
    sessionStorage.removeItem('ct-token')
  }
}

function getAuthToken(): string | null {
  return sessionStorage.getItem('ct-token')
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  }
  const token = getAuthToken()
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }
  const res = await fetch(path, {
    ...options,
    headers: {
      ...headers,
      ...options.headers as Record<string, string>,
    },
  })

  if (res.status === 401) {
    onUnauthorizedCallback?.()
    throw new ApiClientError(401, 'Unauthorized')
  }

  if (!res.ok) {
    const body = await res.json().catch(() => ({ message: res.statusText })) as { message?: string }
    throw new ApiClientError(res.status, body.message ?? res.statusText)
  }

  if (res.status === 204) {
    return undefined as T
  }

  return res.json() as Promise<T>
}

// Auth
export async function login(data: LoginRequest): Promise<LoginResponse> {
  return request('/api/auth/login', { method: 'POST', body: JSON.stringify(data) })
}

export async function logout(): Promise<void> {
  return request('/api/auth/logout', { method: 'POST' })
}

// Users
export async function getUsers(): Promise<User[]> {
  return request('/api/users')
}

export async function createUser(data: { username: string; password: string }): Promise<User> {
  return request('/api/users', { method: 'POST', body: JSON.stringify(data) })
}

export async function deleteUser(id: string): Promise<void> {
  return request(`/api/users/${id}`, { method: 'DELETE' })
}

// API Tokens
export async function getTokens(): Promise<ApiToken[]> {
  return request('/api/tokens')
}

export async function createToken(data: { name: string }): Promise<ApiTokenCreated> {
  return request('/api/tokens', { method: 'POST', body: JSON.stringify(data) })
}

export async function revokeToken(id: string): Promise<void> {
  return request(`/api/tokens/${id}`, { method: 'DELETE' })
}

// Templates
interface ApiTemplateResponse {
  id: string
  name: string
  is_default: boolean
  roles: SessionTemplate['roles']
  mappings: ApiMapping[]
  created_at: string
  updated_at: string
}

function fromApiTemplate(t: ApiTemplateResponse): SessionTemplate {
  return {
    ...t,
    mappings: (t.mappings ?? []).map(apiToMapping),
  }
}

function toApiTemplateBody(data: SessionTemplateCreate): string {
  return JSON.stringify({
    name: data.name,
    is_default: data.is_default,
    roles: data.roles,
    mappings: data.mappings.map(mappingToApi),
  })
}

export async function getTemplates(): Promise<SessionTemplate[]> {
  const raw: ApiTemplateResponse[] = await request('/api/templates')
  return raw.map(fromApiTemplate)
}

export async function getTemplate(id: string): Promise<SessionTemplate> {
  const raw: ApiTemplateResponse = await request(`/api/templates/${id}`)
  return fromApiTemplate(raw)
}

export async function createTemplate(data: SessionTemplateCreate): Promise<SessionTemplate> {
  const raw: ApiTemplateResponse = await request('/api/templates', { method: 'POST', body: toApiTemplateBody(data) })
  return fromApiTemplate(raw)
}

export async function updateTemplate(id: string, data: SessionTemplateCreate): Promise<SessionTemplate> {
  const raw: ApiTemplateResponse = await request(`/api/templates/${id}`, { method: 'PUT', body: toApiTemplateBody(data) })
  return fromApiTemplate(raw)
}

export async function deleteTemplate(id: string): Promise<void> {
  return request(`/api/templates/${id}`, { method: 'DELETE' })
}

// Sessions
export async function getSessions(): Promise<Session[]> {
  return request('/api/sessions')
}

export async function getSession(id: string): Promise<SessionDetail> {
  return request(`/api/sessions/${id}`)
}

export async function createSession(data: SessionCreateRequest): Promise<Session> {
  return request('/api/sessions', { method: 'POST', body: JSON.stringify(data) })
}

export async function endSession(id: string): Promise<void> {
  return request(`/api/sessions/${id}`, { method: 'DELETE' })
}

export async function assignSession(id: string, data: { peer_id: string; role: string }): Promise<void> {
  return request(`/api/sessions/${id}/assign`, { method: 'POST', body: JSON.stringify(data) })
}

export interface PeerConnection {
  id: string
  session_id?: string
  role?: string
  client_id?: string
}

export async function getConnections(): Promise<PeerConnection[]> {
  return request('/api/connections')
}

// Clients
export async function getClients(): Promise<Client[]> {
  return request('/api/clients')
}

export async function getClient(id: string): Promise<Client> {
  return request(`/api/clients/${id}`)
}

// Server Status
export async function getServerStatus(): Promise<ServerStatus> {
  return request('/api/status')
}

export { ApiClientError }
