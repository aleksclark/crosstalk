import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { SessionDetailPage } from './SessionDetailPage'

const mockNavigate = vi.fn()

vi.mock('@/lib/api/client', () => ({
  getSession: vi.fn().mockResolvedValue({
    id: 'session-1',
    name: 'Test Session',
    template_id: 'tmpl-1',
    template_name: 'Translation',
    status: 'active',
    client_count: 2,
    total_roles: 2,
    created_at: '2026-04-21T10:00:00Z',
    ended_at: null,
    clients: [
      { id: 'client-1', role: 'translator', status: 'connected', connected_at: '2026-04-21T10:00:00Z' },
      { id: 'client-2', role: 'studio', status: 'connected', connected_at: '2026-04-21T10:05:00Z' },
    ],
    channel_bindings: [
      { from_role: 'translator', from_channel: 'mic', to_role: 'studio', to_channel: 'output', active: true },
    ],
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

describe('SessionDetailPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
  })

  it('renders session metadata', async () => {
    render(
      <MemoryRouter initialEntries={['/sessions/session-1']}>
        <Routes>
          <Route path="/sessions/:id" element={<SessionDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByText('Test Session')).toBeInTheDocument()
      expect(screen.getByText(/Translation/)).toBeInTheDocument()
      expect(screen.getByText('active')).toBeInTheDocument()
      expect(screen.getByText('2 / 2 clients connected')).toBeInTheDocument()
    })
  })

  it('renders connected clients table', async () => {
    render(
      <MemoryRouter initialEntries={['/sessions/session-1']}>
        <Routes>
          <Route path="/sessions/:id" element={<SessionDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByText('client-1')).toBeInTheDocument()
      expect(screen.getByText('client-2')).toBeInTheDocument()
      expect(screen.getByText('translator')).toBeInTheDocument()
      expect(screen.getByText('studio')).toBeInTheDocument()
    })
  })

  it('end session button calls API', async () => {
    const { endSession } = await import('@/lib/api/client')

    render(
      <MemoryRouter initialEntries={['/sessions/session-1']}>
        <Routes>
          <Route path="/sessions/:id" element={<SessionDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByTestId('end-session-button')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByTestId('end-session-button'))

    await waitFor(() => {
      expect(endSession).toHaveBeenCalledWith('session-1')
    })
  })

  it('renders channel bindings', async () => {
    render(
      <MemoryRouter initialEntries={['/sessions/session-1']}>
        <Routes>
          <Route path="/sessions/:id" element={<SessionDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByText('translator:mic')).toBeInTheDocument()
      expect(screen.getByText('studio:output')).toBeInTheDocument()
      expect(screen.getByText('Active')).toBeInTheDocument()
    })
  })
})
