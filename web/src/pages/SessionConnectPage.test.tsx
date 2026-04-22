import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { SessionConnectPage } from './SessionConnectPage'

const mockNavigate = vi.fn()

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

// Mock navigator.mediaDevices
Object.defineProperty(navigator, 'mediaDevices', {
  value: {
    getUserMedia: vi.fn().mockRejectedValue(new Error('Not available in test')),
    enumerateDevices: vi.fn().mockResolvedValue([]),
  },
  writable: true,
  configurable: true,
})

describe('SessionConnectPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders session connect view with audio controls and debug panel', async () => {
    render(
      <MemoryRouter initialEntries={['/sessions/session-1/connect?role=translator']}>
        <Routes>
          <Route path="/sessions/:id/connect" element={<SessionConnectPage />} />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByText('Session: Test Session')).toBeInTheDocument()
      expect(screen.getByText('Role: translator')).toBeInTheDocument()
    })

    // Audio controls
    expect(screen.getByTestId('mic-section')).toBeInTheDocument()
    expect(screen.getByTestId('mic-device-select')).toBeInTheDocument()
    expect(screen.getByTestId('mic-mute-button')).toBeInTheDocument()
    expect(screen.getByTestId('mic-vu-meter')).toBeInTheDocument()

    // WebRTC debug
    expect(screen.getByTestId('webrtc-debug')).toBeInTheDocument()

    // Session logs
    expect(screen.getByTestId('session-logs')).toBeInTheDocument()
    expect(screen.getByTestId('log-filter')).toBeInTheDocument()
  })

  it('renders end session button', async () => {
    render(
      <MemoryRouter initialEntries={['/sessions/session-1/connect']}>
        <Routes>
          <Route path="/sessions/:id/connect" element={<SessionConnectPage />} />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByTestId('end-session-button')).toBeInTheDocument()
    })
  })
})
