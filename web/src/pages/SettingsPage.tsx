import { useState, useEffect } from 'react'
import { getUsers, createUser, deleteUser, getTokens, createToken, revokeToken } from '@/lib/api/client'
import type { User, ApiToken, ApiTokenCreated } from '@/lib/api/types'
import { Button } from '@/components/ui/button'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export function SettingsPage() {
  const [users, setUsers] = useState<User[]>([])
  const [tokens, setTokens] = useState<ApiToken[]>([])
  const [loading, setLoading] = useState(true)

  const [showCreateUser, setShowCreateUser] = useState(false)
  const [newUsername, setNewUsername] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [creatingUser, setCreatingUser] = useState(false)
  const [userError, setUserError] = useState('')

  const [showCreateToken, setShowCreateToken] = useState(false)
  const [newTokenName, setNewTokenName] = useState('')
  const [creatingToken, setCreatingToken] = useState(false)
  const [tokenError, setTokenError] = useState('')
  const [createdToken, setCreatedToken] = useState<ApiTokenCreated | null>(null)

  useEffect(() => {
    void Promise.all([getUsers(), getTokens()])
      .then(([u, t]) => {
        setUsers(u)
        setTokens(t)
      })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [])

  const handleCreateUser = async () => {
    if (!newUsername.trim() || !newPassword.trim()) return
    setCreatingUser(true)
    setUserError('')
    try {
      const user = await createUser({ username: newUsername, password: newPassword })
      setUsers((prev) => [...prev, user])
      setNewUsername('')
      setNewPassword('')
      setShowCreateUser(false)
    } catch (err) {
      setUserError(err instanceof Error ? err.message : 'Failed to create user')
    } finally {
      setCreatingUser(false)
    }
  }

  const handleDeleteUser = async (id: string) => {
    if (!confirm('Delete this user?')) return
    try {
      await deleteUser(id)
      setUsers((prev) => prev.filter((u) => u.id !== id))
    } catch {
      // silently handle
    }
  }

  const handleCreateToken = async () => {
    if (!newTokenName.trim()) return
    setCreatingToken(true)
    setTokenError('')
    try {
      const result = await createToken({ name: newTokenName })
      setCreatedToken(result)
      setTokens((prev) => [...prev, { id: result.id, name: result.name, created_at: new Date().toISOString(), last_used_at: null }])
      setNewTokenName('')
      setShowCreateToken(false)
    } catch (err) {
      setTokenError(err instanceof Error ? err.message : 'Failed to create token')
    } finally {
      setCreatingToken(false)
    }
  }

  const handleRevokeToken = async (id: string) => {
    if (!confirm('Revoke this token? This cannot be undone.')) return
    try {
      await revokeToken(id)
      setTokens((prev) => prev.filter((t) => t.id !== id))
    } catch {
      // silently handle
    }
  }

  const handleCopyToken = (token: string) => {
    void navigator.clipboard.writeText(token)
  }

  if (loading) return <div className="text-muted-foreground">Loading...</div>

  return (
    <div className="space-y-8 max-w-4xl">
      <h1 className="text-3xl font-bold text-foreground">Settings</h1>

      {/* --- Users --- */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>Users</CardTitle>
            <Button
              size="sm"
              onClick={() => { setShowCreateUser(!showCreateUser); setUserError('') }}
              data-testid="toggle-create-user"
            >
              {showCreateUser ? 'Cancel' : 'Create User'}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {showCreateUser && (
            <div className="border border-border rounded-md p-4 space-y-3" data-testid="create-user-form">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-1">
                  <Label htmlFor="new-username">Username</Label>
                  <Input
                    id="new-username"
                    value={newUsername}
                    onChange={(e) => setNewUsername(e.target.value)}
                    placeholder="Username"
                    data-testid="new-username-input"
                  />
                </div>
                <div className="space-y-1">
                  <Label htmlFor="new-password">Password</Label>
                  <Input
                    id="new-password"
                    type="password"
                    value={newPassword}
                    onChange={(e) => setNewPassword(e.target.value)}
                    placeholder="Password"
                    data-testid="new-password-input"
                  />
                </div>
              </div>
              {userError && (
                <div className="text-sm text-destructive" data-testid="user-error">{userError}</div>
              )}
              <Button
                size="sm"
                onClick={handleCreateUser}
                disabled={creatingUser || !newUsername.trim() || !newPassword.trim()}
                data-testid="confirm-create-user"
              >
                {creatingUser ? 'Creating...' : 'Create'}
              </Button>
            </div>
          )}

          {users.length === 0 ? (
            <p className="text-muted-foreground text-sm">No users</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Username</TableHead>
                  <TableHead>Created</TableHead>
                  <TableHead>Actions</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {users.map((user) => (
                  <TableRow key={user.id} data-testid="user-row">
                    <TableCell className="font-medium">{user.username}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {new Date(user.created_at).toLocaleString()}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleDeleteUser(user.id)}
                        data-testid="delete-user-button"
                      >
                        Delete
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>

      {/* --- API Tokens --- */}
      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>API Tokens</CardTitle>
            <Button
              size="sm"
              onClick={() => { setShowCreateToken(!showCreateToken); setTokenError(''); setCreatedToken(null) }}
              data-testid="toggle-create-token"
            >
              {showCreateToken ? 'Cancel' : 'Create Token'}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          {createdToken && (
            <div className="border border-success/30 bg-success/5 rounded-md p-4 space-y-2" data-testid="token-created-banner">
              <div className="text-sm font-medium text-foreground">
                Token created — copy it now, it will not be shown again.
              </div>
              <div className="flex items-center gap-2">
                <code className="flex-1 bg-muted px-3 py-2 rounded text-xs font-mono break-all" data-testid="created-token-value">
                  {createdToken.token}
                </code>
                <Button size="sm" variant="outline" onClick={() => handleCopyToken(createdToken.token)} data-testid="copy-token-button">
                  Copy
                </Button>
              </div>
              <Button size="sm" variant="ghost" onClick={() => setCreatedToken(null)}>
                Dismiss
              </Button>
            </div>
          )}

          {showCreateToken && (
            <div className="border border-border rounded-md p-4 space-y-3" data-testid="create-token-form">
              <div className="space-y-1">
                <Label htmlFor="new-token-name">Token Name</Label>
                <Input
                  id="new-token-name"
                  value={newTokenName}
                  onChange={(e) => setNewTokenName(e.target.value)}
                  placeholder="e.g. CI Pipeline"
                  data-testid="new-token-name-input"
                />
              </div>
              {tokenError && (
                <div className="text-sm text-destructive" data-testid="token-error">{tokenError}</div>
              )}
              <Button
                size="sm"
                onClick={handleCreateToken}
                disabled={creatingToken || !newTokenName.trim()}
                data-testid="confirm-create-token"
              >
                {creatingToken ? 'Creating...' : 'Create'}
              </Button>
            </div>
          )}

          {tokens.length === 0 ? (
            <p className="text-muted-foreground text-sm">No API tokens</p>
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
                  <TableRow key={token.id} data-testid="token-row">
                    <TableCell className="font-medium">{token.name}</TableCell>
                    <TableCell className="font-mono text-xs text-muted-foreground">{token.id}</TableCell>
                    <TableCell className="text-xs text-muted-foreground">
                      {new Date(token.created_at).toLocaleString()}
                    </TableCell>
                    <TableCell>
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => handleRevokeToken(token.id)}
                        data-testid="revoke-token-button"
                      >
                        Revoke
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
