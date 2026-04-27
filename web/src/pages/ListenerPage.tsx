import { useEffect } from 'react'
import { useParams, useSearchParams } from 'react-router-dom'
import { useBroadcastListener } from '@/lib/use-broadcast-listener'
import { Button } from '@/components/ui/button'

export function ListenerPage() {
  const { sessionId } = useParams<{ sessionId: string }>()
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''

  const listener = useBroadcastListener({
    sessionId: sessionId ?? '',
    token,
  })

  // Auto-connect on mount when we have a valid token
  useEffect(() => {
    if (sessionId && token && listener.status === 'idle') {
      listener.connect()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, token])

  // Show error if no token provided
  if (!token) {
    return (
      <div className="min-h-screen bg-background flex items-center justify-center p-4" data-testid="listener-page">
        <div className="text-center space-y-4 max-w-sm w-full">
          <div className="text-4xl">🔇</div>
          <p className="text-destructive text-lg" data-testid="error-message">
            Invalid or expired link
          </p>
          <p className="text-muted-foreground text-sm">
            Please scan the QR code again or request a new link.
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-background flex items-center justify-center p-4" data-testid="listener-page">
      <div className="text-center space-y-8 max-w-sm w-full">
        {/* Session Title */}
        {listener.sessionName && (
          <div className="space-y-2">
            <div className="text-3xl">🎵</div>
            <h1 className="text-2xl font-bold text-foreground" data-testid="session-title">
              {listener.sessionName}
            </h1>
          </div>
        )}

        {/* Error State */}
        {listener.status === 'error' && (
          <div className="space-y-4">
            <div className="text-4xl">🔇</div>
            <p className="text-destructive text-lg" data-testid="error-message">
              {listener.error ?? 'Something went wrong'}
            </p>
            <p className="text-muted-foreground text-sm">
              Please scan the QR code again or request a new link.
            </p>
          </div>
        )}

        {/* Loading State */}
        {(listener.status === 'loading' || listener.status === 'connecting') && (
          <div className="space-y-4">
            <div className="text-4xl animate-pulse">📡</div>
            <p className="text-muted-foreground" data-testid="connection-status">
              {listener.status === 'loading' ? 'Loading...' : 'Connecting...'}
            </p>
          </div>
        )}

        {/* Connected / Disconnected State */}
        {(listener.status === 'connected' || listener.status === 'disconnected') && (
          <div className="space-y-8">
            {/* Play / Pause Button */}
            <Button
              size="lg"
              className="w-24 h-24 rounded-full text-3xl"
              onClick={listener.togglePlayPause}
              data-testid="play-pause-button"
            >
              {listener.isPlaying ? '⏸' : '▶'}
            </Button>

            {/* Volume Slider */}
            <div className="space-y-2 px-4">
              <div className="flex items-center gap-3">
                <span className="text-muted-foreground text-sm">🔈</span>
                <input
                  type="range"
                  min="0"
                  max="1"
                  step="0.01"
                  value={listener.volume}
                  onChange={(e) => listener.setVolume(parseFloat(e.target.value))}
                  className="flex-1 h-2 accent-primary"
                  data-testid="volume-slider"
                />
                <span className="text-muted-foreground text-sm">🔊</span>
              </div>
            </div>

            {/* Connection Status */}
            <div className="flex items-center justify-center gap-2" data-testid="connection-status">
              <span
                className={`inline-block w-2 h-2 rounded-full ${
                  listener.status === 'connected' ? 'bg-success' : 'bg-warning'
                }`}
              />
              <span className="text-sm text-muted-foreground">
                {listener.status === 'connected' ? 'Connected' : 'Disconnected'}
              </span>
            </div>
          </div>
        )}

        {/* Footer */}
        <div className="pt-8">
          <p className="text-xs text-muted-foreground/50">
            Powered by CrossTalk
          </p>
        </div>
      </div>
    </div>
  )
}
