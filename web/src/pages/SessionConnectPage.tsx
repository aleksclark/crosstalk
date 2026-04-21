import { useState, useEffect } from 'react'
import { useParams, useSearchParams, useNavigate } from 'react-router-dom'
import { getSession, endSession } from '@/lib/api/client'
import type { SessionDetail } from '@/lib/api/types'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Select } from '@/components/ui/select'
import { Badge } from '@/components/ui/badge'

interface LogEntry {
  timestamp: string
  source: string
  message: string
  severity: 'debug' | 'info' | 'warn' | 'error'
}

export function SessionConnectPage() {
  const { id } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const role = searchParams.get('role') ?? ''

  const [session, setSession] = useState<SessionDetail | null>(null)
  const [loading, setLoading] = useState(true)

  // Audio state
  const [devices, setDevices] = useState<MediaDeviceInfo[]>([])
  const [selectedDevice, setSelectedDevice] = useState('')
  const [muted, setMuted] = useState(false)

  // WebRTC debug state (placeholder)
  const [iceState] = useState('new')
  const [stats] = useState({
    localCandidates: 0,
    remoteCandidates: 0,
    bytesSent: 0,
    bytesReceived: 0,
    packetLoss: 0,
    jitter: 0,
    rtt: 0,
  })

  // Session logs
  const [logs] = useState<LogEntry[]>([
    { timestamp: new Date().toISOString(), source: 'system', message: 'Waiting for WebRTC connection...', severity: 'info' },
  ])
  const [logFilter, setLogFilter] = useState<string>('all')

  useEffect(() => {
    if (!id) return
    void getSession(id)
      .then(setSession)
      .catch(() => navigate('/sessions'))
      .finally(() => setLoading(false))
  }, [id, navigate])

  // Enumerate audio devices
  useEffect(() => {
    async function enumerateDevices() {
      try {
        // Request permission first to get device labels
        await navigator.mediaDevices.getUserMedia({ audio: true })
        const allDevices = await navigator.mediaDevices.enumerateDevices()
        const audioInputs = allDevices.filter((d) => d.kind === 'audioinput')
        setDevices(audioInputs)
        if (audioInputs.length > 0 && !selectedDevice) {
          setSelectedDevice(audioInputs[0].deviceId)
        }
      } catch {
        // Mic permission denied or not available
      }
    }
    void enumerateDevices()
  }, [selectedDevice])

  const handleEndSession = async () => {
    if (!id || !confirm('End this session?')) return
    await endSession(id)
    navigate('/sessions')
  }

  const filteredLogs = logFilter === 'all' ? logs : logs.filter((l) => l.severity === logFilter)

  if (loading || !session) return <div className="text-muted-foreground">Loading...</div>

  return (
    <div className="space-y-4">
      {/* Header bar */}
      <div className="flex items-center justify-between bg-card border border-border rounded-lg p-4">
        <div className="flex items-center gap-4">
          <span className="text-foreground font-semibold">Session: {session.name}</span>
          {role && (
            <Badge variant="secondary">Role: {role}</Badge>
          )}
          <Badge variant={iceState === 'connected' ? 'success' : iceState === 'failed' ? 'destructive' : 'warning'}>
            ICE: {iceState}
          </Badge>
        </div>
        <Button variant="destructive" onClick={handleEndSession} data-testid="end-session-button">
          End Session
        </Button>
      </div>

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        {/* Audio Channels Panel (Left) */}
        <Card>
          <CardHeader>
            <CardTitle>Audio Channels</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            {/* Incoming channels placeholder */}
            <div className="space-y-3" data-testid="incoming-channels">
              <div className="text-sm text-muted-foreground">Incoming Channels</div>
              <div className="border border-border rounded-md p-3 space-y-2">
                <div className="text-xs text-muted-foreground">No incoming channels yet</div>
                {/* Channel placeholders for when connected */}
                <div className="space-y-2" data-testid="channel-list">
                  {/* Each channel would be rendered here */}
                </div>
              </div>
            </div>

            {/* Mic section */}
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

              {/* VU Meter placeholder */}
              <div className="space-y-1" data-testid="mic-vu-meter">
                <div className="h-4 bg-muted rounded-full overflow-hidden">
                  <div className="h-full bg-success rounded-full transition-all" style={{ width: '0%' }} />
                </div>
                <div className="text-xs text-muted-foreground">-∞ dBFS</div>
              </div>
            </div>

            {/* Volume controls placeholder */}
            <div className="space-y-3 border-t border-border pt-4" data-testid="volume-controls">
              <div className="text-sm text-muted-foreground">Volume Controls</div>
              <div className="text-xs text-muted-foreground">Connect to a session to see volume controls</div>
            </div>
          </CardContent>
        </Card>

        {/* WebRTC Debug Panel (Right) */}
        <Card>
          <CardHeader>
            <CardTitle>WebRTC Debug</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="space-y-2 text-sm font-mono" data-testid="webrtc-debug">
              <div className="flex justify-between">
                <span className="text-muted-foreground">ICE State</span>
                <span className="text-foreground">{iceState}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">ICE Candidates</span>
                <span className="text-foreground">{stats.localCandidates} local, {stats.remoteCandidates} remote</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Bytes Sent</span>
                <span className="text-foreground">{formatBytes(stats.bytesSent)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Bytes Received</span>
                <span className="text-foreground">{formatBytes(stats.bytesReceived)}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Packet Loss</span>
                <span className="text-foreground">{stats.packetLoss.toFixed(1)}%</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">Jitter</span>
                <span className="text-foreground">{stats.jitter}ms</span>
              </div>
              <div className="flex justify-between">
                <span className="text-muted-foreground">RTT</span>
                <span className="text-foreground">{stats.rtt}ms</span>
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      {/* Session Logs Panel (Bottom) */}
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
