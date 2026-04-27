import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { getSession, getTemplate, endSession, getConnections, assignSession } from '@/lib/api/client'
import type { PeerConnection } from '@/lib/api/client'
import type { SessionDetail, Role, Mapping } from '@/lib/api/types'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Select } from '@/components/ui/select'
import { BroadcastCard } from '@/components/BroadcastCard'

export function SessionDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const [session, setSession] = useState<SessionDetail | null>(null)
  const [loading, setLoading] = useState(true)
  const [peers, setPeers] = useState<PeerConnection[]>([])
  const [templateRoles, setTemplateRoles] = useState<Role[]>([])
  const [connectRole, setConnectRole] = useState('')
  const [assignRole, setAssignRole] = useState('studio')
  const [assignError, setAssignError] = useState('')
  const [templateMappings, setTemplateMappings] = useState<Mapping[]>([])

  useEffect(() => {
    if (!id) return
    void getSession(id)
      .then((s) => {
        setSession(s)
        if (s.template_id) {
          void getTemplate(s.template_id)
            .then((t) => {
              setTemplateRoles(t.roles)
              setTemplateMappings(t.mappings)
              if (t.roles.length > 0) {
                setConnectRole(t.roles[0].name)
                setAssignRole(t.roles[0].name)
              }
            })
            .catch(() => {})
        }
      })
      .catch(() => navigate('/sessions'))
      .finally(() => setLoading(false))
  }, [id, navigate])

  useEffect(() => {
    if (!id) return
    const load = () => void getConnections().then(setPeers).catch(() => {})
    load()
    const iv = setInterval(load, 3000)
    return () => clearInterval(iv)
  }, [id])

  const handleEnd = async () => {
    if (!id || !confirm('End this session?')) return
    await endSession(id)
    setSession((prev) => (prev ? { ...prev, status: 'ended' } : prev))
  }

  if (loading || !session) return <div className="text-muted-foreground">Loading...</div>

  return (
    <div className="space-y-6 max-w-4xl">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-3xl font-bold text-foreground">{session.name}</h1>
          <p className="text-muted-foreground text-sm">
            Template: {session.template_name} · Created: {new Date(session.created_at).toLocaleString()}
          </p>
        </div>
        <div className="flex items-center gap-2">
          {session.status !== 'ended' && (
            <>
              {templateRoles.length > 0 && (
                <Select
                  value={connectRole}
                  onChange={(e) => setConnectRole(e.target.value)}
                  className="w-36 h-9"
                  data-testid="connect-role-select"
                >
                  {templateRoles.map((r) => (
                    <option key={r.name} value={r.name}>{r.name}</option>
                  ))}
                </Select>
              )}
              <Button
                variant="outline"
                onClick={() => navigate(`/sessions/${id}/connect?role=${encodeURIComponent(connectRole)}`)}
                data-testid="connect-button"
              >
                Connect
              </Button>
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
        <span className="text-sm text-muted-foreground">
          {session.client_count} / {session.total_roles} clients connected
        </span>
      </div>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>Assign Peers</CardTitle>
            <Badge variant="secondary">{peers.length} online</Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-2">
          {peers.map((p) => (
            <div key={p.id} className="flex items-center justify-between border border-border rounded-md p-2">
              <div className="flex items-center gap-2">
                <span className="font-mono text-xs">{p.id.slice(0, 12)}...</span>
                {p.role && <Badge variant="secondary">{p.role}</Badge>}
                {p.session_id === id && <Badge variant="success">in session</Badge>}
                {p.session_id && p.session_id !== id && <Badge variant="warning">other session</Badge>}
              </div>
              {(!p.session_id || p.session_id !== id) && session.status !== 'ended' && (
                <div className="flex items-center gap-2">
                  <Select value={assignRole} onChange={(e) => setAssignRole(e.target.value)} className="w-28 h-8 text-xs" data-testid="assign-role-select">
                    {templateRoles.map((r) => (
                      <option key={r.name} value={r.name}>{r.name}</option>
                    ))}
                  </Select>
                  <Button size="sm" variant="outline" data-testid="assign-peer-button" onClick={async () => {
                    setAssignError('')
                    try {
                      await assignSession(id!, { peer_id: p.id, role: assignRole })
                      const updated = await getConnections()
                      setPeers(updated)
                    } catch (err) {
                      setAssignError(err instanceof Error ? err.message : 'Assign failed')
                    }
                  }}>Assign</Button>
                </div>
              )}
            </div>
          ))}
          {assignError && <div className="text-sm text-destructive" data-testid="assign-error">{assignError}</div>}
          {peers.length === 0 && <p className="text-muted-foreground text-sm">No peers connected to server</p>}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Connected Clients</CardTitle>
        </CardHeader>
        <CardContent>
          {(session.clients?.length ?? 0) === 0 ? (
            <p className="text-muted-foreground text-sm">No clients connected</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Client ID</TableHead>
                  <TableHead>Role</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Connected Since</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(session.clients ?? []).map((client) => (
                  <TableRow key={client.id}>
                    <TableCell className="font-mono text-xs">{client.id}</TableCell>
                    <TableCell>{client.role}</TableCell>
                    <TableCell>
                      <Badge variant={client.status === 'connected' ? 'success' : 'secondary'}>
                        {client.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-xs">{new Date(client.connected_at).toLocaleString()}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {session.status !== 'ended' && (
        <BroadcastCard
          sessionId={id!}
          hasBroadcastMapping={templateMappings.some((m) => m.to_type === 'broadcast')}
          listenerCount={session.listener_count}
        />
      )}

      <Card>
        <CardHeader>
          <CardTitle>Channel Bindings</CardTitle>
        </CardHeader>
        <CardContent>
          {(session.channel_bindings?.length ?? 0) === 0 ? (
            <p className="text-muted-foreground text-sm">No channel bindings</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Source</TableHead>
                  <TableHead>Target</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(session.channel_bindings ?? []).map((binding, i) => (
                  <TableRow key={i}>
                    <TableCell>{binding.from_role}:{binding.from_channel}</TableCell>
                    <TableCell>{binding.to_role}:{binding.to_channel}</TableCell>
                    <TableCell>
                      <Badge variant={binding.active ? 'success' : 'secondary'}>
                        {binding.active ? 'Active' : 'Inactive'}
                      </Badge>
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
