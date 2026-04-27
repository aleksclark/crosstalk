import { useRef, useState, useCallback, useEffect } from 'react'
import { getBroadcastInfo } from '@/lib/api/client'
import type { SignalMessage } from './webrtc-types'

export type BroadcastListenerStatus = 'idle' | 'loading' | 'connecting' | 'connected' | 'disconnected' | 'error'

interface UseBroadcastListenerOptions {
  sessionId: string
  token: string
}

interface UseBroadcastListenerReturn {
  status: BroadcastListenerStatus
  sessionName: string
  error: string | null
  volume: number
  setVolume: (v: number) => void
  isPlaying: boolean
  togglePlayPause: () => void
  connect: () => void
  disconnect: () => void
}

export function useBroadcastListener(options: UseBroadcastListenerOptions): UseBroadcastListenerReturn {
  const { sessionId, token } = options

  const [status, setStatus] = useState<BroadcastListenerStatus>('idle')
  const [sessionName, setSessionName] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [volume, setVolumeState] = useState(1)
  const [isPlaying, setIsPlaying] = useState(false)

  const wsRef = useRef<WebSocket | null>(null)
  const pcRef = useRef<RTCPeerConnection | null>(null)
  const audioCtxRef = useRef<AudioContext | null>(null)
  const gainNodeRef = useRef<GainNode | null>(null)
  const audioElRef = useRef<HTMLAudioElement | null>(null)
  const connectedRef = useRef(false)
  const reachedConnectedRef = useRef(false)

  const setVolume = useCallback((v: number) => {
    setVolumeState(v)
    if (gainNodeRef.current) {
      gainNodeRef.current.gain.value = v
    }
    if (audioElRef.current) {
      audioElRef.current.volume = v
    }
  }, [])

  const togglePlayPause = useCallback(() => {
    const ctx = audioCtxRef.current
    if (!ctx) return

    if (ctx.state === 'running') {
      void ctx.suspend()
      setIsPlaying(false)
    } else {
      void ctx.resume().then(() => setIsPlaying(true))
    }
  }, [])

  const disconnect = useCallback(() => {
    connectedRef.current = false
    reachedConnectedRef.current = false

    if (pcRef.current) {
      pcRef.current.close()
      pcRef.current = null
    }

    if (wsRef.current) {
      wsRef.current.close()
      wsRef.current = null
    }

    if (audioCtxRef.current) {
      void audioCtxRef.current.close()
      audioCtxRef.current = null
    }

    if (audioElRef.current) {
      audioElRef.current.pause()
      audioElRef.current.srcObject = null
      audioElRef.current.remove()
      audioElRef.current = null
    }

    gainNodeRef.current = null
    setIsPlaying(false)
  }, [])

  const connect = useCallback(() => {
    if (connectedRef.current) return
    connectedRef.current = true

    setStatus('loading')
    setError(null)

    void (async () => {
      // Step 1: Fetch session info (public endpoint, no auth)
      let iceServers: RTCIceServer[] = [{ urls: 'stun:stun.l.google.com:19302' }]
      try {
        const info = await getBroadcastInfo(sessionId)
        setSessionName(info.session_name)
        if (info.ice_servers && info.ice_servers.length > 0) {
          iceServers = info.ice_servers
        }
      } catch (err) {
        const msg = err instanceof Error ? err.message : 'Failed to load session'
        setError(msg)
        setStatus('error')
        connectedRef.current = false
        return
      }

      // Step 2: Open WebSocket
      setStatus('connecting')

      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const wsUrl = `${protocol}//${window.location.host}/ws/broadcast?token=${encodeURIComponent(token)}`
      const ws = new WebSocket(wsUrl)
      wsRef.current = ws

      // Step 3: Create PeerConnection
      const pc = new RTCPeerConnection({ iceServers })
      pcRef.current = pc

      // Add recvonly audio transceiver
      pc.addTransceiver('audio', { direction: 'recvonly' })

      pc.oniceconnectionstatechange = () => {
        const state = pc.iceConnectionState
        if (state === 'connected' || state === 'completed') {
          reachedConnectedRef.current = true
          setStatus('connected')
        } else if (state === 'disconnected' || state === 'closed') {
          setStatus('disconnected')
        } else if (state === 'failed') {
          setError('Connection failed')
          setStatus('error')
        }
      }

      pc.onicecandidate = (event) => {
        if (event.candidate && ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: 'ice', candidate: event.candidate.toJSON() }))
        }
      }

      pc.ontrack = (event) => {
        if (event.track.kind !== 'audio') return

        // Set up audio context + gain node for volume control
        const ctx = new AudioContext()
        audioCtxRef.current = ctx

        const stream = event.streams[0] || new MediaStream([event.track])
        const source = ctx.createMediaStreamSource(stream)
        const gain = ctx.createGain()
        gain.gain.value = volume

        source.connect(gain)
        gain.connect(ctx.destination)
        gainNodeRef.current = gain

        // Create a hidden audio element for reliable playback
        const audioEl = document.createElement('audio')
        audioEl.srcObject = new MediaStream([event.track])
        audioEl.autoplay = true
        audioEl.style.display = 'none'
        document.body.appendChild(audioEl)
        audioElRef.current = audioEl

        // Resume audio context (may be suspended due to autoplay policy)
        void ctx.resume().then(() => setIsPlaying(true))
      }

      // Handle renegotiation (server-initiated offers)
      let initialOfferSent = false
      pc.onnegotiationneeded = async () => {
        if (!initialOfferSent) return
        try {
          if (pc.signalingState !== 'stable') return
          const offer = await pc.createOffer()
          await pc.setLocalDescription(offer)
          if (ws.readyState === WebSocket.OPEN) {
            ws.send(JSON.stringify({ type: 'offer', sdp: offer.sdp }))
          }
        } catch {
          // Renegotiation failed
        }
      }

      ws.onopen = async () => {
        try {
          const offer = await pc.createOffer()
          await pc.setLocalDescription(offer)
          ws.send(JSON.stringify({ type: 'offer', sdp: offer.sdp }))
          initialOfferSent = true
        } catch {
          setError('Failed to create WebRTC offer')
          setStatus('error')
        }
      }

      ws.onmessage = async (event) => {
        try {
          const msg = JSON.parse(event.data as string) as SignalMessage

          if (msg.type === 'answer' && msg.sdp) {
            await pc.setRemoteDescription({ type: 'answer', sdp: msg.sdp })
          }

          if (msg.type === 'offer' && msg.sdp) {
            await pc.setRemoteDescription({ type: 'offer', sdp: msg.sdp })
            const answer = await pc.createAnswer()
            await pc.setLocalDescription(answer)
            if (ws.readyState === WebSocket.OPEN) {
              ws.send(JSON.stringify({ type: 'answer', sdp: answer.sdp }))
            }
          }

          if (msg.type === 'ice' && msg.candidate) {
            await pc.addIceCandidate(msg.candidate)
          }
        } catch {
          // Signal message parsing/handling error
        }
      }

      ws.onerror = () => {
        setError('Connection error')
        setStatus('error')
      }

      ws.onclose = (event) => {
        // If we never reached connected state, this is likely an auth error
        if (!reachedConnectedRef.current) {
          if (event.code === 1008 || event.code === 4001) {
            setError('Invalid or expired link')
          } else {
            setError('Connection closed')
          }
          setStatus('error')
        } else {
          setStatus('disconnected')
        }
      }
    })()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sessionId, token, disconnect])

  // Clean up on unmount
  useEffect(() => {
    return () => {
      disconnect()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return {
    status,
    sessionName,
    error,
    volume,
    setVolume,
    isPlaying,
    togglePlayPause,
    connect,
    disconnect,
  }
}
