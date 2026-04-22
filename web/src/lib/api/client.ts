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
} from './types'

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

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const res = await fetch(path, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options.headers,
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
export async function getTemplates(): Promise<SessionTemplate[]> {
  return request('/api/templates')
}

export async function getTemplate(id: string): Promise<SessionTemplate> {
  return request(`/api/templates/${id}`)
}

export async function createTemplate(data: SessionTemplateCreate): Promise<SessionTemplate> {
  return request('/api/templates', { method: 'POST', body: JSON.stringify(data) })
}

export async function updateTemplate(id: string, data: SessionTemplateCreate): Promise<SessionTemplate> {
  return request(`/api/templates/${id}`, { method: 'PUT', body: JSON.stringify(data) })
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
