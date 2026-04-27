import { useState, useEffect, useRef, useCallback } from 'react'
import { QRCodeSVG } from 'qrcode.react'
import { createBroadcastToken } from '@/lib/api/client'
import { Card, CardHeader, CardTitle, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'

interface BroadcastCardProps {
  sessionId: string
  hasBroadcastMapping: boolean
  listenerCount?: number
}

export function BroadcastCard({ sessionId, hasBroadcastMapping, listenerCount = 0 }: BroadcastCardProps) {
  const [broadcastUrl, setBroadcastUrl] = useState<string | null>(null)
  const [expiresAt, setExpiresAt] = useState<Date | null>(null)
  const [countdown, setCountdown] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [copied, setCopied] = useState(false)
  const countdownRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const updateCountdown = useCallback(() => {
    if (!expiresAt) return
    const diff = expiresAt.getTime() - Date.now()
    if (diff <= 0) {
      setCountdown('Expired')
      if (countdownRef.current) {
        clearInterval(countdownRef.current)
        countdownRef.current = null
      }
      return
    }
    const minutes = Math.floor(diff / 60000)
    const seconds = Math.floor((diff % 60000) / 1000)
    setCountdown(`${minutes}:${seconds.toString().padStart(2, '0')}`)
  }, [expiresAt])

  useEffect(() => {
    if (!expiresAt) return
    updateCountdown()
    countdownRef.current = setInterval(updateCountdown, 1000)
    return () => {
      if (countdownRef.current) {
        clearInterval(countdownRef.current)
        countdownRef.current = null
      }
    }
  }, [expiresAt, updateCountdown])

  if (!hasBroadcastMapping) return null

  const handleGenerate = async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await createBroadcastToken(sessionId)
      setBroadcastUrl(result.url)
      setExpiresAt(new Date(result.expires_at))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to generate broadcast link')
    } finally {
      setLoading(false)
    }
  }

  const handleCopy = async () => {
    if (!broadcastUrl) return
    try {
      await navigator.clipboard.writeText(broadcastUrl)
      setCopied(true)
      setTimeout(() => setCopied(false), 2000)
    } catch {
      // Clipboard API may not be available
    }
  }

  return (
    <Card data-testid="broadcast-card">
      <CardHeader>
        <div className="flex items-center justify-between">
          <CardTitle>Broadcast</CardTitle>
          <div className="flex items-center gap-2">
            {listenerCount > 0 && (
              <Badge variant="secondary" data-testid="listener-count">
                🔊 {listenerCount} listener{listenerCount !== 1 ? 's' : ''}
              </Badge>
            )}
            {expiresAt && countdown !== 'Expired' && (
              <Badge variant="outline">
                ⏱ {countdown}
              </Badge>
            )}
            {expiresAt && countdown === 'Expired' && (
              <Badge variant="warning">
                Expired
              </Badge>
            )}
          </div>
        </div>
      </CardHeader>
      <CardContent>
        {error && (
          <div className="text-sm text-destructive mb-4">{error}</div>
        )}

        {!broadcastUrl ? (
          <Button
            onClick={handleGenerate}
            disabled={loading}
            data-testid="generate-link-button"
          >
            {loading ? 'Generating...' : 'Generate Broadcast Link'}
          </Button>
        ) : (
          <div className="space-y-4">
            <div className="flex justify-center" data-testid="qr-code">
              <QRCodeSVG
                value={broadcastUrl}
                size={200}
                bgColor="transparent"
                fgColor="currentColor"
                className="text-foreground"
              />
            </div>

            <div className="flex items-center gap-2">
              <code
                className="flex-1 text-xs bg-muted rounded px-2 py-1.5 overflow-hidden text-ellipsis whitespace-nowrap"
                data-testid="broadcast-url"
              >
                {broadcastUrl}
              </code>
              <Button
                variant="outline"
                size="sm"
                onClick={handleCopy}
                data-testid="copy-link-button"
              >
                {copied ? 'Copied!' : 'Copy'}
              </Button>
            </div>

            <Button
              variant="outline"
              size="sm"
              onClick={handleGenerate}
              disabled={loading}
            >
              {loading ? 'Regenerating...' : 'Regenerate'}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
