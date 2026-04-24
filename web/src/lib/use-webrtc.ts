import { useRef, useState, useCallback, useEffect } from 'react'
import type {
  SignalMessage,
  WebRTCStats,
  AudioChannel,
  ConnectionState,
  LogEntryMessage,
  BindChannelMessage,
} from './webrtc-types'

interface UseWebRTCOptions {
  sessionId: string
  role: string
  token: string
}

interface UseWebRTCReturn {
  iceState: ConnectionState
  stats: WebRTCStats
  channels: AudioChannel[]
  logs: LogEntryMessage[]
  connect: () => void
  disconnect: () => void
  setChannelGain: (channelId: string, gain: number) => void
  setMicStream: (stream: MediaStream | null) => void
  micLevel: number
}

const STATS_INTERVAL = 2000
const EMPTY_STATS: WebRTCStats = {
  localCandidates: 0,
  remoteCandidates: 0,
  bytesSent: 0,
  bytesReceived: 0,
  packetLoss: 0,
  jitter: 0,
  rtt: 0,
}

export function useWebRTC(options: UseWebRTCOptions): UseWebRTCReturn {
  const { sessionId, role, token } = options

  const [iceState, setIceState] = useState<ConnectionState>('new')
  const [stats, setStats] = useState<WebRTCStats>(EMPTY_STATS)
  const [channels, setChannels] = useState<AudioChannel[]>([])
  const [logs, setLogs] = useState<LogEntryMessage[]>([])
  const [micLevel, setMicLevel] = useState(0)

  const wsRef = useRef<WebSocket | null>(null)
  const pcRef = useRef<RTCPeerConnection | null>(null)
  const dcRef = useRef<RTCDataChannel | null>(null)
  const audioCtxRef = useRef<AudioContext | null>(null)
  const gainNodesRef = useRef<Map<string, GainNode>>(new Map())
  const analyserNodesRef = useRef<Map<string, AnalyserNode>>(new Map())
  const micAnalyserRef = useRef<AnalyserNode | null>(null)
  const micSourceRef = useRef<MediaStreamAudioSourceNode | null>(null)
  const statsIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const levelIntervalRef = useRef<ReturnType<typeof setInterval> | null>(null)
  const senderRef = useRef<RTCRtpSender | null>(null)

  const addLog = useCallback((severity: LogEntryMessage['severity'], source: string, message: string) => {
    setLogs((prev) => [...prev, { timestamp: Date.now(), severity, source, message }])
  }, [])

  const sendSignal = useCallback((msg: SignalMessage) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify(msg))
    }
  }, [])

  const sendControl = useCallback((msg: Record<string, unknown>) => {
    if (dcRef.current?.readyState === 'open') {
      dcRef.current.send(JSON.stringify(msg))
    }
  }, [])

  const getOrCreateAudioContext = useCallback(() => {
    if (!audioCtxRef.current) {
      audioCtxRef.current = new AudioContext()
    }
    return audioCtxRef.current
  }, [])

  const handleBindChannel = useCallback((bind: BindChannelMessage) => {
    addLog('info', 'system', `Bind channel: ${bind.localName} (${bind.direction})`)

    setChannels((prev) => {
      const existing = prev.find((c) => c.id === bind.channelId)
      if (existing) return prev
      return [...prev, {
        id: bind.channelId,
        name: bind.localName,
        direction: bind.direction,
        trackId: bind.trackId,
        level: 0,
      }]
    })
  }, [addLog])

  const handleUnbindChannel = useCallback((channelId: string) => {
    addLog('info', 'system', `Unbind channel: ${channelId}`)
    setChannels((prev) => prev.filter((c) => c.id !== channelId))
    gainNodesRef.current.delete(channelId)
    analyserNodesRef.current.delete(channelId)
  }, [addLog])

  const handleTrack = useCallback((event: RTCTrackEvent) => {
    addLog('info', 'webrtc', `Received remote track: ${event.track.kind} (${event.track.id})`)

    const ctx = getOrCreateAudioContext()
    const stream = event.streams[0]
    if (!stream || event.track.kind !== 'audio') return

    // Expose the remote stream for E2E audio capture tests.
    // The golden-audio spec looks for <audio> elements with srcObject
    // or window.__remoteStream.
    ;(window as Record<string, unknown>).__remoteStream = stream
    const audioEl = document.createElement('audio')
    audioEl.srcObject = stream
    audioEl.autoplay = true
    audioEl.style.display = 'none'
    document.body.appendChild(audioEl)

    const source = ctx.createMediaStreamSource(stream)
    const gain = ctx.createGain()
    const analyser = ctx.createAnalyser()
    analyser.fftSize = 256

    source.connect(gain)
    gain.connect(analyser)
    analyser.connect(ctx.destination)

    const trackId = event.track.id
    gainNodesRef.current.set(trackId, gain)
    analyserNodesRef.current.set(trackId, analyser)
  }, [addLog, getOrCreateAudioContext])

  const handleDataChannel = useCallback((event: RTCDataChannelEvent) => {
    const dc = event.channel
    addLog('info', 'webrtc', `Data channel opened: ${dc.label}`)
    dcRef.current = dc

    dc.onmessage = (msgEvent) => {
      try {
        const msg = JSON.parse(msgEvent.data as string) as Record<string, unknown>

        if (msg['welcome']) {
          const welcome = msg['welcome'] as { clientId: string; serverVersion: string }
          addLog('info', 'server', `Welcome: client=${welcome.clientId} server=${welcome.serverVersion}`)

          sendControl({
            joinSession: { sessionId, role },
          })
        }

        if (msg['bindChannel']) {
          handleBindChannel(msg['bindChannel'] as BindChannelMessage)
        }

        if (msg['unbindChannel']) {
          const unbind = msg['unbindChannel'] as { channelId: string }
          handleUnbindChannel(unbind.channelId)
        }

        if (msg['sessionEvent']) {
          const evt = msg['sessionEvent'] as { type: string; message: string }
          addLog('info', 'session', `${evt.type}: ${evt.message}`)
        }

        if (msg['logEntry']) {
          const entry = msg['logEntry'] as LogEntryMessage
          setLogs((prev) => [...prev, entry])
        }
      } catch {
        addLog('warn', 'system', 'Failed to parse data channel message')
      }
    }

    dc.onerror = () => {
      addLog('error', 'webrtc', 'Data channel error')
    }

    dc.onclose = () => {
      addLog('info', 'webrtc', 'Data channel closed')
    }
  }, [addLog, sendControl, sessionId, role, handleBindChannel, handleUnbindChannel])

  const pollStats = useCallback(async () => {
    const pc = pcRef.current
    if (!pc) return

    try {
      const report = await pc.getStats()
      let localCandidates = 0
      let remoteCandidates = 0
      let bytesSent = 0
      let bytesReceived = 0
      let packetLoss = 0
      let jitter = 0
      let rtt = 0

      report.forEach((stat) => {
        if (stat.type === 'local-candidate') localCandidates++
        if (stat.type === 'remote-candidate') remoteCandidates++
        if (stat.type === 'transport') {
          bytesSent += (stat as { bytesSent?: number }).bytesSent ?? 0
          bytesReceived += (stat as { bytesReceived?: number }).bytesReceived ?? 0
        }
        if (stat.type === 'inbound-rtp') {
          const inbound = stat as { packetsLost?: number; packetsReceived?: number; jitter?: number }
          const lost = inbound.packetsLost ?? 0
          const received = inbound.packetsReceived ?? 0
          if (received + lost > 0) {
            packetLoss = (lost / (received + lost)) * 100
          }
          jitter = (inbound.jitter ?? 0) * 1000
        }
        if (stat.type === 'candidate-pair' && (stat as { state?: string }).state === 'succeeded') {
          rtt = (stat as { currentRoundTripTime?: number }).currentRoundTripTime
            ? ((stat as { currentRoundTripTime: number }).currentRoundTripTime * 1000)
            : 0
        }
      })

      setStats({ localCandidates, remoteCandidates, bytesSent, bytesReceived, packetLoss, jitter, rtt })
    } catch {
      // stats unavailable
    }
  }, [])

  const pollLevels = useCallback(() => {
    const dataArray = new Uint8Array(128)

    if (micAnalyserRef.current) {
      micAnalyserRef.current.getByteFrequencyData(dataArray)
      const avg = dataArray.reduce((sum, v) => sum + v, 0) / dataArray.length
      setMicLevel(avg / 255)
    }

    analyserNodesRef.current.forEach((analyser, trackId) => {
      analyser.getByteFrequencyData(dataArray)
      const avg = dataArray.reduce((sum, v) => sum + v, 0) / dataArray.length
      const level = avg / 255

      setChannels((prev) =>
        prev.map((ch) =>
          ch.trackId === trackId ? { ...ch, level } : ch,
        ),
      )
    })
  }, [])

  const connect = useCallback(() => {
    if (pcRef.current) return

    addLog('info', 'system', 'Initiating WebRTC connection...')
    setIceState('connecting')

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsUrl = `${protocol}//${window.location.host}/ws/signaling?token=${encodeURIComponent(token)}`
    const ws = new WebSocket(wsUrl)
    wsRef.current = ws

    const pc = new RTCPeerConnection({
      iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
    })
    pcRef.current = pc

    pc.oniceconnectionstatechange = () => {
      const state = pc.iceConnectionState as ConnectionState
      setIceState(state)
      addLog('debug', 'webrtc', `ICE state: ${state}`)
    }

    pc.onicecandidate = (event) => {
      if (event.candidate) {
        sendSignal({ type: 'ice', candidate: event.candidate.toJSON() })
      }
    }

    pc.ontrack = handleTrack
    pc.ondatachannel = handleDataChannel

    ws.onopen = async () => {
      addLog('info', 'system', 'WebSocket connected, creating offer...')

      try {
        pc.addTransceiver('audio', { direction: 'recvonly' })

        const offer = await pc.createOffer()
        await pc.setLocalDescription(offer)
        sendSignal({ type: 'offer', sdp: offer.sdp })
      } catch (err) {
        addLog('error', 'webrtc', `Failed to create offer: ${err instanceof Error ? err.message : String(err)}`)
      }
    }

    ws.onmessage = async (event) => {
      try {
        const msg = JSON.parse(event.data as string) as SignalMessage

        if (msg.type === 'answer' && msg.sdp) {
          await pc.setRemoteDescription({ type: 'answer', sdp: msg.sdp })
          addLog('debug', 'webrtc', 'Set remote description (answer)')
        }

        if (msg.type === 'offer' && msg.sdp) {
          await pc.setRemoteDescription({ type: 'offer', sdp: msg.sdp })
          const answer = await pc.createAnswer()
          await pc.setLocalDescription(answer)
          sendSignal({ type: 'answer', sdp: answer.sdp })
          addLog('debug', 'webrtc', 'Handled renegotiation offer')
        }

        if (msg.type === 'ice' && msg.candidate) {
          await pc.addIceCandidate(msg.candidate)
        }
      } catch (err) {
        addLog('error', 'webrtc', `Signal error: ${err instanceof Error ? err.message : String(err)}`)
      }
    }

    ws.onerror = () => {
      addLog('error', 'system', 'WebSocket error')
    }

    ws.onclose = () => {
      addLog('info', 'system', 'WebSocket closed')
    }

    statsIntervalRef.current = setInterval(() => void pollStats(), STATS_INTERVAL)
    levelIntervalRef.current = setInterval(pollLevels, 100)
  }, [token, addLog, sendSignal, handleTrack, handleDataChannel, pollStats, pollLevels])

  const disconnect = useCallback(() => {
    if (statsIntervalRef.current) {
      clearInterval(statsIntervalRef.current)
      statsIntervalRef.current = null
    }
    if (levelIntervalRef.current) {
      clearInterval(levelIntervalRef.current)
      levelIntervalRef.current = null
    }

    if (dcRef.current) {
      dcRef.current.close()
      dcRef.current = null
    }

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

    micSourceRef.current = null
    micAnalyserRef.current = null
    senderRef.current = null
    gainNodesRef.current.clear()
    analyserNodesRef.current.clear()

    setIceState('closed')
    setStats(EMPTY_STATS)
    setChannels([])
    setMicLevel(0)
    addLog('info', 'system', 'Disconnected')
  }, [addLog])

  const setChannelGain = useCallback((channelId: string, gain: number) => {
    const node = gainNodesRef.current.get(channelId)
    if (node) {
      node.gain.value = gain
    }
  }, [])

  const setMicStream = useCallback((stream: MediaStream | null) => {
    if (micSourceRef.current) {
      micSourceRef.current.disconnect()
      micSourceRef.current = null
    }
    if (micAnalyserRef.current) {
      micAnalyserRef.current = null
    }

    if (!stream) {
      if (senderRef.current && pcRef.current) {
        pcRef.current.removeTrack(senderRef.current)
        senderRef.current = null
      }
      return
    }

    const ctx = getOrCreateAudioContext()
    const source = ctx.createMediaStreamSource(stream)
    const analyser = ctx.createAnalyser()
    analyser.fftSize = 256
    source.connect(analyser)

    micSourceRef.current = source
    micAnalyserRef.current = analyser

    const track = stream.getAudioTracks()[0]
    if (track && pcRef.current) {
      if (senderRef.current) {
        void senderRef.current.replaceTrack(track)
      } else {
        senderRef.current = pcRef.current.addTrack(track, stream)
      }
    }
  }, [getOrCreateAudioContext])

  useEffect(() => {
    return () => {
      disconnect()
    }
  }, [disconnect])

  return {
    iceState,
    stats,
    channels,
    logs,
    connect,
    disconnect,
    setChannelGain,
    setMicStream,
    micLevel,
  }
}
