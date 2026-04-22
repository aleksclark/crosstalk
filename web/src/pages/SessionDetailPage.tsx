import { useState, useEffect } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { getSession, endSession } from '@/lib/api/client'
import type { Session } from '@/lib/api/types'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'

export function SessionDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [session, setSession] = useState<Session | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!id) return
    void getSession(id)
      .then(setSession)
      .catch(() => navigate('/sessions'))
      .finally(() => setLoading(false))
  }, [id, navigate])

  const handleEnd = async () => {
    if (!id || !confirm('End this session?')) return
    await endSession(id)
    setSession((prev) => (prev ? { ...prev, status: 'ended' as const } : prev))
  }

  if (loading || !session) return <div className="text-muted-foreground">Loading...</div>

  return (
    <div className="space-y-6 max-w-4xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-foreground">{session.name}</h1>
          <p className="text-muted-foreground text-sm">
            Template: {session.template_id} · Created: {new Date(session.created_at!).toLocaleString()}
          </p>
        </div>
        <div className="flex gap-2">
          {session.status !== 'ended' && (
            <>
              <Link to={`/sessions/${id}/connect`}>
                <Button variant="outline">Connect</Button>
              </Link>
              <Button variant="destructive" onClick={handleEnd} data-testid="end-session-button">
                End Session
              </Button>
            </>
          )}
        </div>
      </div>

      <div className="flex gap-3 items-center">
        <Badge variant={session.status === 'active' ? 'success' : session.status === 'waiting' ? 'warning' : 'secondary'}>
          {session.status}
        </Badge>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Session Info</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-muted-foreground">ID</span>
            <span className="font-mono text-xs">{session.id}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">Template ID</span>
            <span className="font-mono text-xs">{session.template_id}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">Status</span>
            <span>{session.status}</span>
          </div>
          <div className="flex justify-between">
            <span className="text-muted-foreground">Created</span>
            <span>{new Date(session.created_at!).toLocaleString()}</span>
          </div>
          {session.ended_at && (
            <div className="flex justify-between">
              <span className="text-muted-foreground">Ended</span>
              <span>{new Date(session.ended_at).toLocaleString()}</span>
            </div>
          )}
          {session.recording && (
            <div className="flex justify-between">
              <span className="text-muted-foreground">Recording</span>
              <span>{session.recording.active ? 'Active' : 'Inactive'} — {session.recording.file_count ?? 0} files</span>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
