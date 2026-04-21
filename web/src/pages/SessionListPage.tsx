import { useState, useEffect } from 'react'
import { Link, useNavigate } from 'react-router-dom'
import { getSessions, getTemplates, createSession, endSession } from '@/lib/api/client'
import type { Session, SessionTemplate } from '@/lib/api/types'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Select } from '@/components/ui/select'
import { Input } from '@/components/ui/input'

function statusBadgeVariant(status: string): 'success' | 'warning' | 'secondary' {
  switch (status) {
    case 'active': return 'success'
    case 'waiting': return 'warning'
    default: return 'secondary'
  }
}

export function SessionListPage() {
  const [sessions, setSessions] = useState<Session[]>([])
  const [templates, setTemplates] = useState<SessionTemplate[]>([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [newName, setNewName] = useState('')
  const [newTemplateId, setNewTemplateId] = useState('')
  const [creating, setCreating] = useState(false)
  const navigate = useNavigate()

  useEffect(() => {
    void Promise.all([getSessions(), getTemplates()]).then(([s, t]) => {
      setSessions(s)
      setTemplates(t)
      setLoading(false)
    }).catch(() => setLoading(false))
  }, [])

  const handleCreate = async () => {
    if (!newTemplateId || !newName.trim()) return
    setCreating(true)
    try {
      const session = await createSession({ template_id: newTemplateId, name: newName })
      setSessions((prev) => [session, ...prev])
      setShowCreate(false)
      setNewName('')
      setNewTemplateId('')
    } catch {
      // error handling
    } finally {
      setCreating(false)
    }
  }

  const handleEnd = async (id: string) => {
    if (!confirm('End this session?')) return
    await endSession(id)
    setSessions((prev) => prev.map((s) => (s.id === id ? { ...s, status: 'ended' as const } : s)))
  }

  if (loading) return <div className="text-muted-foreground">Loading...</div>

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold text-foreground">Sessions</h1>
        <Button onClick={() => setShowCreate(!showCreate)} data-testid="create-session-button">
          Create Session
        </Button>
      </div>

      {showCreate && (
        <Card>
          <CardHeader>
            <CardTitle>New Session</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex items-end gap-4">
              <div className="flex-1 space-y-1">
                <label className="text-sm text-muted-foreground">Name</label>
                <Input
                  value={newName}
                  onChange={(e) => setNewName(e.target.value)}
                  placeholder="Session name"
                  data-testid="session-name-input"
                />
              </div>
              <div className="w-48 space-y-1">
                <label className="text-sm text-muted-foreground">Template</label>
                <Select
                  value={newTemplateId}
                  onChange={(e) => setNewTemplateId(e.target.value)}
                  data-testid="session-template-select"
                >
                  <option value="">Select template</option>
                  {templates.map((t) => (
                    <option key={t.id} value={t.id}>{t.name}</option>
                  ))}
                </Select>
              </div>
              <Button onClick={handleCreate} disabled={creating || !newName.trim() || !newTemplateId} data-testid="confirm-create-session">
                {creating ? 'Creating...' : 'Create'}
              </Button>
            </div>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>All Sessions</CardTitle>
        </CardHeader>
        <CardContent>
          {sessions.length === 0 ? (
            <p className="text-muted-foreground text-sm">No sessions</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Template</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Clients</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead>Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sessions.map((session) => (
                  <TableRow key={session.id} data-testid="session-row">
                    <TableCell>
                      <Link to={`/sessions/${session.id}`} className="text-primary hover:underline">
                        {session.name}
                      </Link>
                    </TableCell>
                    <TableCell>{session.template_name}</TableCell>
                    <TableCell>
                      <Badge variant={statusBadgeVariant(session.status)}>{session.status}</Badge>
                    </TableCell>
                    <TableCell>{session.client_count} / {session.total_roles}</TableCell>
                    <TableCell className="text-xs">{new Date(session.created_at).toLocaleString()}</TableCell>
                    <TableCell>
                      <div className="flex gap-2">
                        {session.status !== 'ended' && (
                          <>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => navigate(`/sessions/${session.id}/connect`)}
                            >
                              Connect
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleEnd(session.id)}
                              data-testid="end-session-button"
                            >
                              End
                            </Button>
                          </>
                        )}
                      </div>
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
