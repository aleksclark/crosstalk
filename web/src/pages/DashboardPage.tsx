import { useState, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { getClients, getSessions, getTemplates, createSession } from '@/lib/api/client'
import type { Session, SessionTemplate } from '@/lib/api/types'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'

export function DashboardPage() {
  const [clientCount, setClientCount] = useState(0)
  const [sessions, setSessions] = useState<Session[]>([])
  const [templates, setTemplates] = useState<SessionTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [quickTestLoading, setQuickTestLoading] = useState(false)
  const navigate = useNavigate()
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    let cancelled = false

    async function fetchData() {
      try {
        const [c, s, t] = await Promise.all([
          getClients(),
          getSessions(),
          getTemplates(),
        ])
        if (cancelled) return
        setClientCount(c.length)
        setSessions(s)
        setTemplates(t)
      } catch {
        // silently handle - auth context will redirect on 401
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
    try {
      const now = new Date().toISOString().replace('T', ' ').slice(0, 16)
      const session = await createSession({
        template_id: defaultTemplate.id,
        name: `Quick Test ${now}`,
      })
      navigate(`/sessions/${session.id}/connect?role=translator`)
    } catch {
      // error handling
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
          {quickTestLoading ? 'Creating...' : 'Quick Test'}
        </Button>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
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
              {clientCount}
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
      </div>
    </div>
  )
}
