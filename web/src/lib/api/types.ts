/**
 * Re-exports from the generated OpenAPI types.
 * DO NOT EDIT — regenerate with: task generate:api
 */
import type { components } from './generated'

export type User = components['schemas']['HttpUserResponse']
export type ApiToken = components['schemas']['HttpTokenResponse']
export type ApiTokenCreated = components['schemas']['HttpTokenCreateResponse']
export type Role = components['schemas']['ServerRole']
export type Mapping = components['schemas']['ServerMapping']
export type SessionTemplate = components['schemas']['HttpTemplateResponse']
export type SessionTemplateCreate = components['schemas']['HttpTemplateRequest']
export type Session = components['schemas']['HttpSessionResponseOAPI']
export type RecordingInfo = components['schemas']['ServerRecordingInfo']
export type LoginRequest = components['schemas']['HttpLoginRequest']
export type LoginResponse = components['schemas']['HttpLoginResponse']
export type ApiError = components['schemas']['HttpErrorEnvelope']

export type SessionStatus = NonNullable<Session['status']>

export interface SessionCreateRequest {
  template_id: string
  name: string
}
