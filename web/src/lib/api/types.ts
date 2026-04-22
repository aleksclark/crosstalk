/* Domain types matching the server REST API */

export interface User {
  id: string
  username: string
  created_at: string
}

export interface ApiToken {
  id: string
  name: string
  created_at: string
  last_used_at: string | null
}

export interface ApiTokenCreate {
  name: string
}

export interface ApiTokenCreated {
  id: string
  name: string
  token: string
}

export interface Role {
  name: string
  multi_client: boolean
}

export interface Mapping {
  from_role: string
  from_channel: string
  to_role: string
  to_channel: string
  to_type: 'role' | 'record' | 'broadcast'
}

export interface SessionTemplate {
  id: string
  name: string
  is_default: boolean
  roles: Role[]
  mappings: Mapping[]
  created_at: string
  updated_at: string
}

export interface SessionTemplateCreate {
  name: string
  is_default: boolean
  roles: Role[]
  mappings: Mapping[]
}

export type SessionStatus = 'waiting' | 'active' | 'ended'

export interface Session {
  id: string
  name: string
  template_id: string
  template_name: string
  status: SessionStatus
  client_count: number
  total_roles: number
  created_at: string
  ended_at: string | null
}

export interface SessionDetail extends Session {
  clients: SessionClient[]
  channel_bindings: ChannelBinding[]
}

export interface SessionClient {
  id: string
  role: string
  status: string
  connected_at: string
}

export interface ChannelBinding {
  from_role: string
  from_channel: string
  to_role: string
  to_channel: string
  active: boolean
}

export interface Client {
  id: string
  role: string
  session: string
  sources: string[]
  sinks: string[]
  codecs: string[]
  status: string
  connected_at: string
}

export interface ServerStatus {
  uptime: number
  active_sessions: number
  connected_clients: number
  version: string
}

export interface LoginRequest {
  username: string
  password: string
}

export interface LoginResponse {
  user: User
}

export interface SessionCreateRequest {
  template_id: string
  name: string
}

export interface ApiError {
  error: string
  message: string
}
