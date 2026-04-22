/**
 * Type-safe API client built on openapi-fetch.
 * DO NOT EDIT — regenerate types with: task generate:api
 */
import createClient from 'openapi-fetch'
import type { paths } from './generated'
import type {
  User,
  ApiToken,
  ApiTokenCreated,
  SessionTemplate,
  SessionTemplateCreate,
  Session,
  LoginResponse,
  SessionCreateRequest,
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

const client = createClient<paths>({ baseUrl: '' })

client.use({
  async onRequest({ request }) {
    request.headers.set('Content-Type', 'application/json')
    const token = getAuthToken()
    if (token) {
      request.headers.set('Authorization', `Bearer ${token}`)
    }
    return request
  },
  async onResponse({ response }) {
    if (response.status === 401) {
      onUnauthorizedCallback?.()
      throw new ApiClientError(401, 'Unauthorized')
    }
    if (!response.ok) {
      const body = await response.clone().json().catch(() => ({ error: { message: response.statusText } })) as { error?: { message?: string } }
      throw new ApiClientError(response.status, body?.error?.message ?? response.statusText)
    }
    return response
  },
})

// --- Auth ---

export async function login(data: { username: string; password: string }): Promise<LoginResponse> {
  const { data: result } = await client.POST('/api/auth/login', { body: data })
  return result!
}

export async function logout(): Promise<void> {
  await client.POST('/api/auth/logout')
}

// --- Users ---

export async function getUsers(): Promise<User[]> {
  const { data: result } = await client.GET('/api/users')
  return result ?? []
}

export async function createUser(data: { username: string; password: string }): Promise<User> {
  const { data: result } = await client.POST('/api/users', { body: data })
  return result!
}

export async function deleteUser(id: string): Promise<void> {
  await client.DELETE('/api/users/{id}', { params: { path: { id } } })
}

// --- API Tokens ---

export async function getTokens(): Promise<ApiToken[]> {
  const { data: result } = await client.GET('/api/tokens')
  return result ?? []
}

export async function createToken(data: { name: string }): Promise<ApiTokenCreated> {
  const { data: result } = await client.POST('/api/tokens', { body: data })
  return result!
}

export async function revokeToken(id: string): Promise<void> {
  await client.DELETE('/api/tokens/{id}', { params: { path: { id } } })
}

// --- Templates ---

export async function getTemplates(): Promise<SessionTemplate[]> {
  const { data: result } = await client.GET('/api/templates')
  return result ?? []
}

export async function getTemplate(id: string): Promise<SessionTemplate> {
  const { data: result } = await client.GET('/api/templates/{id}', { params: { path: { id } } })
  return result!
}

export async function createTemplate(data: SessionTemplateCreate): Promise<SessionTemplate> {
  const { data: result } = await client.POST('/api/templates', { body: data })
  return result!
}

export async function updateTemplate(id: string, data: SessionTemplateCreate): Promise<SessionTemplate> {
  const { data: result } = await client.PUT('/api/templates/{id}', { params: { path: { id } }, body: data })
  return result!
}

export async function deleteTemplate(id: string): Promise<void> {
  await client.DELETE('/api/templates/{id}', { params: { path: { id } } })
}

// --- Sessions ---

export async function getSessions(): Promise<Session[]> {
  const { data: result } = await client.GET('/api/sessions')
  return result ?? []
}

export async function getSession(id: string): Promise<Session> {
  const { data: result } = await client.GET('/api/sessions/{id}', { params: { path: { id } } })
  return result!
}

export async function createSession(data: SessionCreateRequest): Promise<Session> {
  const { data: result } = await client.POST('/api/sessions', { body: data })
  return result!
}

export async function endSession(id: string): Promise<void> {
  await client.DELETE('/api/sessions/{id}', { params: { path: { id } } })
}

// --- Clients (stubs) ---

export async function getClients(): Promise<Record<string, never>[]> {
  const { data: result } = await client.GET('/api/clients')
  return result ?? []
}

export { ApiClientError }
