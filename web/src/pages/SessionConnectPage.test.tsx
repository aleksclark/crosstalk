import { describe, it, expect, vi, beforeEach, type Mock } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { SessionConnectPage } from './SessionConnectPage'

const mockNavigate = vi.fn()
const mockConnect = vi.fn()
const mockDisconnect = vi.fn()
const mockSetMicStream = vi.fn()
const mockSetChannelGain = vi.fn()

let mockWebRTCState = {
  iceState: 'new' as string,
  stats: {
    localCandidates: 0,
    remoteCandidates: 0,
    bytesSent: 0,
    bytesReceived: 0,
    packetLoss: 0,
    jitter: 0,
    rtt: 0,
  },
  channels: [] as Array<{ id: string; name: string; direction: string; trackId: string; level: number }>,
  logs: [] as Array<{ timestamp: number; severity: string; source: string; message: string }>,
  micLevel: 0,
  connect: mockConnect,
  disconnect: mockDisconnect,
  setMicStream: mockSetMicStream,
  setChannelGain: mockSetChannelGain,
}

vi.mock('@/lib/use-webrtc', () => ({
  useWebRTC: () => mockWebRTCState,
}))

vi.mock('@/lib/api/client', () => ({
  getSession: vi.fn().mockResolvedValue({
    id: 'session-1',
    name: 'Test Session',
    template_id: 'tmpl-1',
    template_name: 'Translation',
    status: 'active',
    client_count: 1,
    total_roles: 2,
    created_at: '2026-04-21T10:00:00Z',
    ended_at: null,
    clients: [
      { id: 'client-1', role: 'translator', status: 'connected', connected_at: '2026-04-21T10:00:00Z' },
    ],
    channel_bindings: [],
  }),
  endSession: vi.fn().mockResolvedValue(undefined),
}))

vi.mock('@/lib/use-auth', () => ({
  useAuth: () => ({
    user: { id: '1', username: 'admin', created_at: '' },
    isAuthenticated: true,
    login: vi.fn(),
    logout: vi.fn(),
  }),
}))

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

Object.defineProperty(navigator, 'mediaDevices', {
  value: {
    getUserMedia: vi.fn().mockRejectedValue(new Error('Not available in test')),
    enumerateDevices: vi.fn().mockResolvedValue([]),
  },
  writable: true,
  configurable: true,
})

function renderConnect(route = '/sessions/session-1/connect?role=translator') {
  return render(
    <MemoryRouter initialEntries={[route]}>
      <Routes>
        <Route path="/sessions/:id/connect" element={<SessionConnectPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('SessionConnectPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    sessionStorage.setItem('ct-token', 'test-token')
    mockWebRTCState = {
      iceState: 'new',
      stats: {
        localCandidates: 0,
        remoteCandidates: 0,
        bytesSent: 0,
        bytesReceived: 0,
        packetLoss: 0,
        jitter: 0,
        rtt: 0,
      },
      channels: [],
      logs: [],
      micLevel: 0,
      connect: mockConnect,
      disconnect: mockDisconnect,
      setMicStream: mockSetMicStream,
      setChannelGain: mockSetChannelGain,
    }
  })

  it('renders session connect view with audio controls and debug panel', async () => {
    renderConnect()

    await waitFor(() => {
      expect(screen.getByText('Session: Test Session')).toBeInTheDocument()
      expect(screen.getByText('Role: translator')).toBeInTheDocument()
    })

    expect(screen.getByTestId('mic-section')).toBeInTheDocument()
    expect(screen.getByTestId('mic-device-select')).toBeInTheDocument()
    expect(screen.getByTestId('mic-mute-button')).toBeInTheDocument()
    expect(screen.getByTestId('mic-vu-meter')).toBeInTheDocument()
    expect(screen.getByTestId('webrtc-debug')).toBeInTheDocument()
    expect(screen.getByTestId('session-logs')).toBeInTheDocument()
    expect(screen.getByTestId('log-filter')).toBeInTheDocument()
  })

  it('connect is called after session loads', async () => {
    renderConnect()

    await waitFor(() => {
      expect(screen.getByText('Session: Test Session')).toBeInTheDocument()
    })

    expect(mockConnect).toHaveBeenCalled()
  })

  it('displays VU meter level from webrtc hook', async () => {
    mockWebRTCState.micLevel = 0.5
    renderConnect()

    await waitFor(() => {
      expect(screen.getByText('Session: Test Session')).toBeInTheDocument()
    })

    const vuMeter = screen.getByTestId('mic-vu-meter')
    const bar = vuMeter.querySelector('[style]')
    expect(bar).toBeTruthy()
    expect(bar?.getAttribute('style')).toContain('50%')
  })

  it('displays WebRTC stats in debug panel', async () => {
    mockWebRTCState.iceState = 'connected'
    mockWebRTCState.stats = {
      localCandidates: 3,
      remoteCandidates: 2,
      bytesSent: 1024,
      bytesReceived: 2048,
      packetLoss: 1.5,
      jitter: 10,
      rtt: 25,
    }
    renderConnect()

    await waitFor(() => {
      expect(screen.getByText('Session: Test Session')).toBeInTheDocument()
    })

    const debug = screen.getByTestId('webrtc-debug')
    expect(debug).toHaveTextContent('connected')
    expect(debug).toHaveTextContent('3 local, 2 remote')
    expect(debug).toHaveTextContent('1 KB')
    expect(debug).toHaveTextContent('2 KB')
    expect(debug).toHaveTextContent('1.5%')
    expect(debug).toHaveTextContent('10ms')
    expect(debug).toHaveTextContent('25ms')
  })

  it('displays session log entries from webrtc hook', async () => {
    mockWebRTCState.logs = [
      { timestamp: Date.now(), severity: 'info', source: 'system', message: 'Initiating WebRTC connection...' },
      { timestamp: Date.now(), severity: 'debug', source: 'webrtc', message: 'ICE state: connected' },
    ]
    renderConnect()

    await waitFor(() => {
      expect(screen.getByText('Session: Test Session')).toBeInTheDocument()
    })

    const logs = screen.getByTestId('session-logs')
    expect(logs).toHaveTextContent('Initiating WebRTC connection...')
    expect(logs).toHaveTextContent('ICE state: connected')
  })

  it('end session disconnects webrtc and calls API', async () => {
    vi.spyOn(window, 'confirm').mockReturnValue(true)
    const { endSession } = await import('@/lib/api/client')
    renderConnect()

    await waitFor(() => {
      expect(screen.getByTestId('end-session-button')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByTestId('end-session-button'))

    await waitFor(() => {
      expect(mockDisconnect).toHaveBeenCalled()
      expect(endSession).toHaveBeenCalledWith('session-1')
      expect(mockNavigate).toHaveBeenCalledWith('/sessions')
    })
  })

  it('renders incoming channels with VU meters when tracks arrive', async () => {
    mockWebRTCState.channels = [
      { id: 'ch-1', name: 'translator-mic', direction: 'SINK', trackId: 'track-1', level: 0.7 },
    ]
    renderConnect()

    await waitFor(() => {
      expect(screen.getByText('Session: Test Session')).toBeInTheDocument()
    })

    expect(screen.getAllByText('translator-mic').length).toBeGreaterThan(0)
    const vu = screen.getByTestId('vu-ch-1')
    const bar = vu.querySelector('[style]')
    expect(bar?.getAttribute('style')).toContain('70%')
  })

  it('mic getUserMedia is called on mount for device enumeration', async () => {
    renderConnect()

    await waitFor(() => {
      expect(screen.getByText('Session: Test Session')).toBeInTheDocument()
    })

    expect((navigator.mediaDevices.getUserMedia as Mock)).toHaveBeenCalledWith({ audio: true })
  })
})
