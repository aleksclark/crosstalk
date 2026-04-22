export interface SignalMessage {
  type: 'offer' | 'answer' | 'ice'
  sdp?: string
  candidate?: RTCIceCandidateInit
}

export interface ControlMessage {
  hello?: HelloMessage
  joinSession?: JoinSessionMessage
  welcome?: WelcomeMessage
  bindChannel?: BindChannelMessage
  unbindChannel?: UnbindChannelMessage
  sessionEvent?: SessionEventMessage
  logEntry?: LogEntryMessage
}

export interface HelloMessage {
  sources: Array<{ name: string; type: string }>
  sinks: Array<{ name: string; type: string }>
  codecs: Array<{ name: string; mediaType: string }>
}

export interface JoinSessionMessage {
  sessionId: string
  role: string
}

export interface WelcomeMessage {
  clientId: string
  serverVersion: string
}

export interface BindChannelMessage {
  channelId: string
  localName: string
  direction: 'SOURCE' | 'SINK'
  trackId: string
}

export interface UnbindChannelMessage {
  channelId: string
}

export interface SessionEventMessage {
  type: string
  message: string
  sessionId: string
}

export interface LogEntryMessage {
  timestamp: number
  severity: 'debug' | 'info' | 'warn' | 'error'
  source: string
  message: string
}

export interface WebRTCStats {
  localCandidates: number
  remoteCandidates: number
  bytesSent: number
  bytesReceived: number
  packetLoss: number
  jitter: number
  rtt: number
}

export interface AudioChannel {
  id: string
  name: string
  direction: 'SOURCE' | 'SINK'
  trackId: string
  level: number
}

export type ConnectionState = 'new' | 'connecting' | 'connected' | 'disconnected' | 'failed' | 'closed'
