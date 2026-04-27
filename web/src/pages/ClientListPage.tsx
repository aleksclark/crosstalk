// @ts-nocheck
import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { getClients, getConnections, deleteClient } from '@/lib/api/client'
import type { Client, ConnectedClient } from '@/lib/api/types'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'

export function ClientListPage() {
  const [clients, setClients] = useState<Client[]>([])
  const [connections, setConnections] = useState<ConnectedClient[]>([])
  const [loading, setLoading] = useState(true)
  const navigate = useNavigate()

  useEffect(() => {
    void Promise.all([getClients(), getConnections()])
      .then(([c, conn]) => {
        setClients(c)
        setConnections(conn)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const onlineClientIds = new Set(connections.map((c) => c.client_id).filter(Boolean))

  const handleDelete = async (id: string) => {
    if (!confirm('Delete this client and all its associated tokens?')) return
    try {
      await deleteClient(id)
      setClients((prev) => prev.filter((c) => c.id !== id))
    } catch {
      // silently handle
    }
  }

  if (loading) return <div className="text-muted-foreground">Loading...</div>

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-3xl font-bold text-foreground">Clients</h1>
        <Button onClick={() => navigate('/clients/new')} data-testid="create-client-button">
          Create Client
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Registered Clients</CardTitle>
        </CardHeader>
        <CardContent>
          {clients.length === 0 ? (
            <p className="text-muted-foreground text-sm">No clients registered</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Name</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Source Device</TableHead>
                  <TableHead>Sink Device</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead>Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {clients.map((c) => (
                  <TableRow key={c.id} data-testid="client-row">
                    <TableCell>
                      <button
                        className="text-primary hover:underline font-medium text-left"
                        onClick={() => navigate(`/clients/${c.id}`)}
                      >
                        {c.name}
                      </button>
                    </TableCell>
                    <TableCell>
                      <Badge variant={onlineClientIds.has(c.id) ? 'success' : 'secondary'}>
                        {onlineClientIds.has(c.id) ? 'Online' : 'Offline'}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground font-mono">
                      {c.source_name || '—'}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground font-mono">
                      {c.sink_name || '—'}
                    </TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {new Date(c.created_at).toLocaleString()}
                    </TableCell>
                    <TableCell>
                      <div className="flex gap-2">
                        <Button variant="ghost" size="sm" onClick={() => navigate(`/clients/${c.id}`)}>
                          Edit
                        </Button>
                        <Button variant="ghost" size="sm" onClick={() => handleDelete(c.id)} data-testid="delete-client-button">
                          Delete
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Live Connections</CardTitle>
        </CardHeader>
        <CardContent>
          {connections.length === 0 ? (
            <p className="text-muted-foreground text-sm">No active WebRTC connections</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Peer ID</TableHead>
                  <TableHead>Client</TableHead>
                  <TableHead>Session</TableHead>
                  <TableHead>Role</TableHead>
                  <TableHead>Connected Since</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {connections.map((conn) => {
                  const linkedClient = clients.find((c) => c.id === conn.client_id)
                  return (
                    <TableRow key={conn.id} data-testid="connection-row">
                      <TableCell className="font-mono text-xs">{conn.id.slice(0, 12)}...</TableCell>
                      <TableCell>
                        {linkedClient ? (
                          <button
                            className="text-primary hover:underline text-sm"
                            onClick={() => navigate(`/clients/${linkedClient.id}`)}
                          >
                            {linkedClient.name}
                          </button>
                        ) : (
                          <span className="text-muted-foreground text-sm">Unlinked</span>
                        )}
                      </TableCell>
                      <TableCell className="font-mono text-xs">{conn.session_id ? `${conn.session_id.slice(0, 12)}...` : '—'}</TableCell>
                      <TableCell>
                        {conn.role ? <Badge variant="secondary">{conn.role}</Badge> : <span className="text-muted-foreground">—</span>}
                      </TableCell>
                      <TableCell className="text-xs">{new Date(conn.connected_at).toLocaleString()}</TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
