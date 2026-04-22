import { useState, useEffect, useCallback, useRef } from 'react'
import { useParams, useSearchParams, useNavigate } from 'react-router-dom'
import { getSession, endSession } from '@/lib/api/client'
import { useWebRTC } from '@/lib/use-webrtc'
import type { SessionDetail } from '@/lib/api/types'
import type { LogEntryMessage } from '@/lib/webrtc-types'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Select } from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'

export function SessionConnectPage() {
  const { id } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const role = searchParams.get('role') ?? ''

  const [session, setSession] = useState<SessionDetail | null>(null)
  const [loading, setLoading] = useState(true)

  const [devices, setDevices] = useState<MediaDeviceInfo[]>([])
  const [selectedDevice, setSelectedDevice] = useState('')
  const [muted, setMuted] = useState(false)
  const micStreamRef = useRef<MediaStream | null>(null)

  const [logFilter, setLogFilter] = useState<string>('all')

  const token = sessionStorage.getItem('ct-token') ?? ''

  const webrtc = useWebRTC({
    sessionId: id ?? '',
    role,
    token,
  })

  useEffect(() => {
    if (!id) return
    void getSession(id)
      .then(setSession)
      .catch(() => navigate('/sessions'))
      .finally(() => setLoading(false))
  }, [id, navigate])

  useEffect(() => {
    let cancelled = false
    async function enumerateDevices() {
      try {
        const stream = await navigator.mediaDevices.getUserMedia({ audio: true })
        if (cancelled) {
          stream.getTracks().forEach((t) => t.stop())
          return
        }
        stream.getTracks().forEach((t) => t.stop())
        const allDevices = await navigator.mediaDevices.enumerateDevices()
        const audioInputs = allDevices.filter((d) => d.kind === 'audioinput')
        setDevices(audioInputs)
        if (audioInputs.length > 0 && !selectedDevice) {
          setSelectedDevice(audioInputs[0].deviceId)
        }
      } catch {
        // Mic permission denied
      }
    }
    void enumerateDevices()
    return () => { cancelled = true }
  }, [selectedDevice])

  const handleConnect = useCallback(() => {
    webrtc.connect()
  }, [webrtc])

  useEffect(() => {
    if (session && token && webrtc.iceState === 'new') {
      handleConnect()
    }
  }, [session, token, webrtc.iceState, handleConnect])

  useEffect(() => {
    if (!selectedDevice || muted) {
      if (micStreamRef.current) {
        micStreamRef.current.getTracks().forEach((t) => t.stop())
        micStreamRef.current = null
        webrtc.setMicStream(null)
      }
      return
    }

    let cancelled = false
    async function acquireMic() {
      try {
        const stream = await navigator.mediaDevices.getUserMedia({
          audio: { deviceId: { exact: selectedDevice } },
        })
        if (cancelled) {
          stream.getTracks().forEach((t) => t.stop())
          return
        }
        micStreamRef.current = stream
        webrtc.setMicStream(stream)
      } catch {
        // device unavailable
      }
    }
    void acquireMic()
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedDevice, muted])

  const handleEndSession = async () => {
    if (!id || !confirm('End this session?')) return
    webrtc.disconnect()
    await endSession(id)
    navigate('/sessions')
  }

  const filteredLogs: LogEntryMessage[] = logFilter === 'all'
    ? webrtc.logs
    : webrtc.logs.filter((l) => l.severity === logFilter)

  if (loading || !session) return <div className="text-muted-foreground">Loading...</div>

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between bg-card border border-border rounded-lg p-4">
        <div className="flex items-center gap-4">
          <span className="text-foreground font-semibold">Session: {session.name}</span>
          {role && (
            <Badge variant="secondary">Role: {role}</Badge>
          )}
          <Badge variant={webrtc.iceState === 'connected' ? 'success' : webrtc.iceState === 'failed' ? 'destructive' : 'warning'}>
            ICE: {webrtc.iceState}
          </Badge>
        </div>
        <Button variant="destructive" onClick={handleEndSession} data-testid="end-session-button">
          End Session
        </Button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card>
          <CardHeader>
            <CardTitle>Audio Channels</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="space-y-3" data-testid="incoming-channels">
              <div className="text-sm text-muted-foreground">Incoming Channels</div>
              <div className="border border-border rounded-md p-3 space-y-2">
                {webrtc.channels.filter((c) => c.direction === 'SINK').length === 0 ? (
                  <div className="text-xs text-muted-foreground">No incoming channels yet</div>
                ) : null}
                <div className="space-y-2" data-testid="channel-list">
                  {webrtc.channels
                    .filter((c) => c.direction === 'SINK')
                    .map((channel) => (
                      <div key={channel.id} className="space-y-1">
                        <div className="flex items-center justify-between">
                          <span className="text-sm">{channel.name}</span>
                          <input
                            type="range"
                            min="0"
                            max="1"
                            step="0.01"
                            defaultValue="1"
                            className="w-24"
                            data-testid={`volume-${channel.id}`}
                            onChange={(e) => webrtc.setChannelGain(channel.id, parseFloat(e.target.value))}
                          />
                        </div>
                        <div className="h-2 bg-muted rounded-full overflow-hidden" data-testid={`vu-${channel.id}`}>
                          <div
                            className="h-full bg-success rounded-full transition-all"
                            style={{ width: `${(channel.level * 100).toFixed(0)}%` }}
                          />
                        </div>
                      </div>
                    ))}
                </div>
              </div>
            </div>

            <div className="space-y-3 border-t border-border pt-4" data-testid="mic-section">
              <div className="text-sm text-muted-foreground">Microphone (Outgoing)</div>
              <div className="flex items-center gap-3">
                <Select
                  value={selectedDevice}
                  onChange={(e) => setSelectedDevice(e.target.value)}
                  className="flex-1"
                  data-testid="mic-device-select"
                >
                  {devices.length === 0 ? (
                    <option value="">No audio devices</option>
                  ) : (
                    devices.map((d) => (
                      <option key={d.deviceId} value={d.deviceId}>
                        {d.label || `Microphone ${d.deviceId.slice(0, 8)}`}
                      </option>
                    ))
                  )}
                </Select>
                <Button
                  variant={muted ? 'destructive' : 'outline'}
                  size="sm"
                  onClick={() => setMuted(!muted)}
                  data-testid="mic-mute-button"
                >
                  {muted ? 'Unmute' : 'Mute'}
                </Button>
              </div>

              <div className="space-y-1" data-testid="mic-vu-meter">
                <div className="h-4 bg-muted rounded-full overflow-hidden">
                  <div
                    className="h-full bg-success rounded-full transition-all"
                    style={{ width: `${(webrtc.micLevel * 100).toFixed(0)}%` }}
                  />
                </div>
                <div className="text-xs text-muted-foreground">
                  {webrtc.micLevel > 0 ? `${(20 * Math.log10(webrtc.micLevel)).toFixed(1)} dBFS` : '-∞ dBFS'}
                </div>
              </div>
            </div>

            <div className="space-y-3 border-t border-border pt-4" data-testid="volume-controls">
              <div className="text-sm text-muted-foreground">Volume Controls</div>
              {webrtc.channels.filter((c) => c.direction === 'SINK').length === 0 ? (
                <div className="text-xs text-muted-foreground">Connect to a session to see volume controls</div>
              ) : (
                webrtc.channels
                  .filter((c) => c.direction === 'SINK')
                  .map((channel) => (
                    <div key={channel.id} className="flex items-center gap-3">
                      <span className="text-sm w-24 truncate">{channel.name}</span>
                      <input
                        type="range"
                        min="0"
                        max="2"
                        step="0.01"
                        defaultValue="1"
                        className="flex-1"
                        onChange={(e) => webrtc.setChannelGain(channel.id, parseFloat(e.target.value))}
                      />
                    </div>
                  ))
              )}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>WebRTC Debug</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2 text-sm font-mono" data-testid="webrtc-debug">
              <div className="flex justify-between">
                <span className="text-muted-foreground">ICE State</span>
                <span className="text-foreground">{webrtc.iceState}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">ICE Candidates</span>
                <span className="text-foreground">{webrtc.stats.localCandidates} local, {webrtc.stats.remoteCandidates} remote</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Bytes Sent</span>
                <span className="text-foreground">{formatBytes(webrtc.stats.bytesSent)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Bytes Received</span>
                <span className="text-foreground">{formatBytes(webrtc.stats.bytesReceived)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Packet Loss</span>
                <span className="text-foreground">{webrtc.stats.packetLoss.toFixed(1)}%</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Jitter</span>
                <span className="text-foreground">{webrtc.stats.jitter.toFixed(0)}ms</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">RTT</span>
                <span className="text-foreground">{webrtc.stats.rtt.toFixed(0)}ms</span>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>Session Logs</CardTitle>
            <div className="flex items-center gap-2">
              <Select
                value={logFilter}
                onChange={(e) => setLogFilter(e.target.value)}
                className="w-28 h-8 text-xs"
                data-testid="log-filter"
              >
                <option value="all">All</option>
                <option value="debug">Debug</option>
                <option value="info">Info</option>
                <option value="warn">Warn</option>
                <option value="error">Error</option>
              </Select>
            </div>
          </div>
        </CardHeader>
        <CardContent>
          <div className="bg-background border border-border rounded-md p-3 h-48 overflow-y-auto font-mono text-xs" data-testid="session-logs">
            {filteredLogs.map((log, i) => (
              <div key={i} className={`py-0.5 ${severityColor(log.severity)}`}>
                <span className="text-muted-foreground">
                  {new Date(log.timestamp).toLocaleTimeString()}
                </span>
                {' '}
                <span className="text-primary">[{log.source}]</span>
                {' '}
                {log.message}
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`
}

function severityColor(severity: string): string {
  switch (severity) {
    case 'error': return 'text-destructive'
    case 'warn': return 'text-warning'
    case 'debug': return 'text-muted-foreground'
    default: return 'text-foreground'
  }
}
