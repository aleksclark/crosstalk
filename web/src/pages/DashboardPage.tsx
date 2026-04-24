import { useState, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { getSessions, getTemplates, getServerStatus, createSession } from '@/lib/api/client'
import type { Session, SessionTemplate, ServerStatus } from '@/lib/api/types'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'

function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400)
  const h = Math.floor((seconds % 86400) / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  if (d > 0) return `${d}d ${h}h ${m}m`
  if (h > 0) return `${h}h ${m}m`
  return `${m}m`
}

export function DashboardPage() {
  const [sessions, setSessions] = useState<Session[]>([])
  const [templates, setTemplates] = useState<SessionTemplate[]>([])
  const [serverStatus, setServerStatus] = useState<ServerStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [quickTestLoading, setQuickTestLoading] = useState(false)
  const [quickTestError, setQuickTestError] = useState('')
  const navigate = useNavigate()
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    let cancelled = false

    async function fetchData() {
      try {
        const [s, t, status] = await Promise.all([
          getSessions(),
          getTemplates(),
          getServerStatus().catch(() => null),
        ])
        if (cancelled) return
        setSessions(s)
        setTemplates(t)
        setServerStatus(status)
      } catch {
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void fetchData()

    intervalRef.current = setInterval(() => {
      void fetchData()
    }, 5000)

    return () => {
      cancelled = true
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
  }, [])

  const defaultTemplate = templates.find((t) => t.is_default)
  const activeSessions = sessions.filter((s) => s.status !== 'ended')

  const handleQuickTest = async () => {
    if (!defaultTemplate) return
    setQuickTestLoading(true)
    setQuickTestError('')
    try {
      const now = new Date().toISOString().replace('T', ' ').slice(0, 16)
      const session = await createSession({
        template_id: defaultTemplate.id,
        name: `Quick Test ${now}`,
      })
      navigate(`/sessions/${session.id}/connect?role=translator`)
    } catch (err) {
      setQuickTestError(err instanceof Error ? err.message : 'Quick test failed')
    } finally {
      setQuickTestLoading(false)
    }
  }

  if (loading) {
    return <div className="text-muted-foreground">Loading...</div>
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold text-foreground">Dashboard</h1>
        <Button
          onClick={handleQuickTest}
          disabled={!defaultTemplate || quickTestLoading}
          title={!defaultTemplate ? 'Set a default template first' : 'Create a quick test session'}
          data-testid="quick-test-button"
        >
          {quickTestLoading ? 'Setting up...' : 'Quick Test'}
        </Button>
      </div>

      {quickTestError && (
        <div className="text-sm text-destructive" data-testid="quick-test-error">{quickTestError}</div>
      )}

      <div className="grid gap-4 md:grid-cols-3 lg:grid-cols-5">
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Active Sessions</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold" data-testid="active-sessions-count">
              {activeSessions.length}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Connected Clients</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold" data-testid="connected-clients-count">
              {serverStatus?.connections ?? 0}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Templates</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold">{templates.length}</div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Server Uptime</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-3xl font-bold" data-testid="server-uptime">
              {serverStatus ? formatUptime(serverStatus.uptime) : '—'}
            </div>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Server Version</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="text-xl font-bold font-mono" data-testid="server-version">
              {serverStatus?.version ?? '—'}
            </div>
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Recent Sessions</CardTitle>
        </CardHeader>
        <CardContent>
          {sessions.length === 0 ? (
            <p className="text-muted-foreground text-sm">No sessions yet</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sessions.slice(0, 10).map((s) => (
                  <TableRow key={s.id}>
                    <TableCell>{s.name}</TableCell>
                    <TableCell>
                      <Badge variant={s.status === 'active' ? 'success' : s.status === 'ended' ? 'secondary' : 'warning'}>
                        {s.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-xs">{new Date(s.created_at).toLocaleString()}</TableCell>
                    <TableCell>
                      <Button size="sm" variant="outline" onClick={() => navigate(`/sessions/${s.id}`)}>
                        View
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
