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

export interface ApiMapping {
  source: string
  sink: string
}

export function mappingToApi(m: Mapping): ApiMapping {
  const source = `${m.from_role}:${m.from_channel}`
  let sink: string
  if (m.to_type === 'record') {
    sink = 'record'
  } else if (m.to_type === 'broadcast') {
    sink = 'broadcast'
  } else {
    sink = `${m.to_role}:${m.to_channel}`
  }
  return { source, sink }
}

export function apiToMapping(m: ApiMapping): Mapping {
  const [from_role, ...fromRest] = m.source.split(':')
  const from_channel = fromRest.join(':')

  if (m.sink === 'record') {
    return { from_role, from_channel, to_role: '', to_channel: '', to_type: 'record' }
  }
  if (m.sink === 'broadcast') {
    return { from_role, from_channel, to_role: '', to_channel: '', to_type: 'broadcast' }
  }

  const [to_role, ...toRest] = m.sink.split(':')
  const to_channel = toRest.join(':')
  return { from_role, from_channel, to_role, to_channel, to_type: 'role' }
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
  listener_count?: number
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
  name?: string
  id: string
  client_id?: string
  role: string
  session: string
  session_id?: string
  sources: string[]
  sinks: string[]
  codecs: string[]
  status: string
  connected_at: string
  source_name?: string
  sink_name?: string
  created_at?: string
  [key: string]: unknown
}

export type ConnectedClient = Client

export interface ServerStatus {
  connections?: number
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
  token: string
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

export interface BroadcastTokenResponse {
  token: string
  url: string
  expires_at: string
}

export interface BroadcastInfo {
  session_id: string
  session_name: string
  ice_servers?: RTCIceServer[]
}
