import { useState, useEffect, useRef } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { getClient, createClient, updateClient, getTokens, createToken, revokeToken, getConnections } from '@/lib/api/client'
import type { ApiToken, ApiTokenCreated, ConnectedClient } from '@/lib/api/types'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'

export function ClientEditorPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const isNew = id === 'new'

  const [name, setName] = useState('')
  const [sourceName, setSourceName] = useState('')
  const [sinkName, setSinkName] = useState('')
  const [loading, setLoading] = useState(!isNew)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const [tokens, setTokens] = useState<ApiToken[]>([])
  const [connections, setConnections] = useState<ConnectedClient[]>([])
  const [showCreateToken, setShowCreateToken] = useState(false)
  const [newTokenName, setNewTokenName] = useState('')
  const [creatingToken, setCreatingToken] = useState(false)
  const [createdToken, setCreatedToken] = useState<ApiTokenCreated | null>(null)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  useEffect(() => {
    if (!isNew && id) {
      void Promise.all([
        getClient(id),
        getTokens(),
        getConnections(),
      ])
        .then(([c, allTokens, conns]) => {
          setName(c.name)
          setSourceName(c.source_name)
          setSinkName(c.sink_name)
          setTokens(allTokens.filter((t) => t.client_id === id))
          setConnections(conns.filter((conn) => conn.client_id === id))
        })
        .catch(() => navigate('/clients'))
        .finally(() => setLoading(false))
    }
  }, [id, isNew, navigate])

  useEffect(() => {
    if (isNew || !id) return
    intervalRef.current = setInterval(() => {
      void getConnections().then((conns) => {
        setConnections(conns.filter((conn) => conn.client_id === id))
      }).catch(() => {})
    }, 5000)
    return () => { if (intervalRef.current) clearInterval(intervalRef.current) }
  }, [id, isNew])

  const handleSave = async () => {
    if (!name.trim()) return
    setSaving(true)
    setError('')
    const data = { name: name.trim(), source_name: sourceName.trim(), sink_name: sinkName.trim() }
    try {
      if (isNew) {
        const created = await createClient(data)
        navigate(`/clients/${created.id}`)
      } else if (id) {
        await updateClient(id, data)
      }
      setError('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  const handleCreateToken = async () => {
    if (!newTokenName.trim() || !id) return
    setCreatingToken(true)
    try {
      const result = await createToken({ name: newTokenName, client_id: id })
      setCreatedToken(result)
      setTokens((prev) => [...prev, { id: result.id, name: result.name, client_id: id, created_at: new Date().toISOString() }])
      setNewTokenName('')
      setShowCreateToken(false)
    } catch {
    } finally {
      setCreatingToken(false)
    }
  }

  const handleRevokeToken = async (tokenId: string) => {
    if (!confirm('Revoke this token? This cannot be undone.')) return
    await revokeToken(tokenId)
    setTokens((prev) => prev.filter((t) => t.id !== tokenId))
  }

  if (loading) return <div className="text-muted-foreground">Loading...</div>

  const isOnline = connections.length > 0

  return (
    <div className="space-y-6 max-w-3xl">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h1 className="text-3xl font-bold text-foreground">
            {isNew ? 'Create Client' : 'Edit Client'}
          </h1>
          {!isNew && (
            <Badge variant={isOnline ? 'success' : 'secondary'} data-testid="connection-status">
              {isOnline ? 'Online' : 'Offline'}
            </Badge>
          )}
        </div>
        <div className="flex gap-2">
          <Button variant="outline" onClick={() => navigate('/clients')}>Cancel</Button>
          <Button onClick={handleSave} disabled={saving || !name.trim()} data-testid="save-client-button">
            {saving ? 'Saving...' : 'Save'}
          </Button>
        </div>
      </div>

      {error && (
        <div className="text-sm text-destructive bg-destructive/10 p-3 rounded-md" role="alert" data-testid="client-error">{error}</div>
      )}

      <Card>
        <CardHeader><CardTitle>General</CardTitle></CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="client-name">Name</Label>
            <Input id="client-name" value={name} onChange={(e) => setName(e.target.value)}
              placeholder="e.g. K2B Booth 1" data-testid="client-name-input" />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader><CardTitle>Device Configuration</CardTitle></CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="source-name">Audio Source (PipeWire/ALSA)</Label>
            <Input id="source-name" value={sourceName} onChange={(e) => setSourceName(e.target.value)}
              placeholder="e.g. alsa_input.platform-snd_aloop.0.analog-stereo"
              className="font-mono text-sm" data-testid="source-name-input" />
            <p className="text-xs text-muted-foreground">PipeWire node name or ALSA device for audio input. Leave blank to auto-detect.</p>
          </div>
          <div className="space-y-2">
            <Label htmlFor="sink-name">Audio Sink (PipeWire/ALSA)</Label>
            <Input id="sink-name" value={sinkName} onChange={(e) => setSinkName(e.target.value)}
              placeholder="e.g. alsa_output.platform-snd_aloop.0.analog-stereo"
              className="font-mono text-sm" data-testid="sink-name-input" />
            <p className="text-xs text-muted-foreground">PipeWire node name or ALSA device for audio output. Leave blank to auto-detect.</p>
          </div>
        </CardContent>
      </Card>

      {!isNew && (
        <>
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle>Connection Status</CardTitle>
                <Badge variant={isOnline ? 'success' : 'secondary'}>
                  {connections.length} active {connections.length === 1 ? 'connection' : 'connections'}
                </Badge>
              </div>
            </CardHeader>
            <CardContent>
              {connections.length === 0 ? (
                <p className="text-muted-foreground text-sm">Client is not connected</p>
              ) : (
                <div className="space-y-4">
                  {connections.map((conn) => (
                    <div key={conn.id} className="border border-border rounded-md p-4 space-y-3" data-testid="connection-detail">
                      <div className="grid grid-cols-2 gap-x-8 gap-y-2 text-sm">
                        <div>
                          <span className="text-muted-foreground">Peer ID</span>
                          <div className="font-mono text-xs">{conn.id}</div>
                        </div>
                        <div>
                          <span className="text-muted-foreground">Connected Since</span>
                          <div className="text-xs">{new Date(conn.connected_at).toLocaleString()}</div>
                        </div>
                        {conn.session_id && (
                          <div>
                            <span className="text-muted-foreground">Session</span>
                            <div className="font-mono text-xs">{conn.session_id}</div>
                          </div>
                        )}
                        {conn.role && (
                          <div>
                            <span className="text-muted-foreground">Role</span>
                            <div><Badge variant="secondary">{conn.role}</Badge></div>
                          </div>
                        )}
                      </div>

                      {((conn.sources && conn.sources.length > 0) || (conn.sinks && conn.sinks.length > 0)) && (
                        <div className="border-t border-border pt-3 space-y-2">
                          <div className="text-sm font-medium text-foreground">Detected Devices</div>
                          {conn.sources && conn.sources.length > 0 && (
                            <div>
                              <span className="text-xs text-muted-foreground">Sources</span>
                              <div className="space-y-1">
                                {conn.sources.map((s, i) => (
                                  <div key={i} className="font-mono text-xs bg-muted px-2 py-1 rounded" data-testid="detected-source">
                                    {s.name} <span className="text-muted-foreground">({s.type})</span>
                                  </div>
                                ))}
                              </div>
                            </div>
                          )}
                          {conn.sinks && conn.sinks.length > 0 && (
                            <div>
                              <span className="text-xs text-muted-foreground">Sinks</span>
                              <div className="space-y-1">
                                {conn.sinks.map((s, i) => (
                                  <div key={i} className="font-mono text-xs bg-muted px-2 py-1 rounded" data-testid="detected-sink">
                                    {s.name} <span className="text-muted-foreground">({s.type})</span>
                                  </div>
                                ))}
                              </div>
                            </div>
                          )}
                        </div>
                      )}

                      {conn.codecs && conn.codecs.length > 0 && (
                        <div className="border-t border-border pt-3">
                          <span className="text-xs text-muted-foreground">Codecs</span>
                          <div className="flex gap-1 mt-1">
                            {conn.codecs.map((c, i) => (
                              <Badge key={i} variant="secondary" className="text-xs font-mono">{c}</Badge>
                            ))}
                          </div>
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle>API Tokens</CardTitle>
                <Button size="sm" onClick={() => { setShowCreateToken(!showCreateToken); setCreatedToken(null) }}
                  data-testid="toggle-create-client-token">
                  {showCreateToken ? 'Cancel' : 'Create Token'}
                </Button>
              </div>
            </CardHeader>
            <CardContent className="space-y-4">
              {createdToken && (
                <div className="border border-success/30 bg-success/5 rounded-md p-4 space-y-2" data-testid="client-token-created-banner">
                  <div className="text-sm font-medium text-foreground">Token created — copy it now, it will not be shown again.</div>
                  <div className="flex items-center gap-2">
                    <code className="flex-1 bg-muted px-3 py-2 rounded text-xs font-mono break-all" data-testid="created-client-token-value">
                      {createdToken.token}
                    </code>
                    <Button size="sm" variant="outline" onClick={() => void navigator.clipboard.writeText(createdToken.token)}>
                      Copy
                    </Button>
                  </div>
                  <Button size="sm" variant="ghost" onClick={() => setCreatedToken(null)}>Dismiss</Button>
                </div>
              )}

              {showCreateToken && (
                <div className="border border-border rounded-md p-4 space-y-3" data-testid="create-client-token-form">
                  <div className="space-y-1">
                    <Label htmlFor="client-token-name">Token Name</Label>
                    <Input id="client-token-name" value={newTokenName}
                      onChange={(e) => setNewTokenName(e.target.value)}
                      placeholder="e.g. K2B deploy token" data-testid="client-token-name-input" />
                  </div>
                  <Button size="sm" onClick={handleCreateToken}
                    disabled={creatingToken || !newTokenName.trim()} data-testid="confirm-create-client-token">
                    {creatingToken ? 'Creating...' : 'Create'}
                  </Button>
                </div>
              )}

              {tokens.length === 0 ? (
                <p className="text-muted-foreground text-sm">No tokens associated with this client</p>
              ) : (
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Name</TableHead>
                      <TableHead>ID</TableHead>
                      <TableHead>Created</TableHead>
                      <TableHead>Actions</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {tokens.map((token) => (
                      <TableRow key={token.id} data-testid="client-token-row">
                        <TableCell className="font-medium">{token.name}</TableCell>
                        <TableCell className="font-mono text-xs text-muted-foreground">{token.id}</TableCell>
                        <TableCell className="text-xs text-muted-foreground">
                          {new Date(token.created_at).toLocaleString()}
                        </TableCell>
                        <TableCell>
                          <Button variant="ghost" size="sm" onClick={() => handleRevokeToken(token.id)}
                            data-testid="revoke-client-token-button">Revoke</Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </CardContent>
          </Card>
        </>
      )}
    </div>
  )
}
