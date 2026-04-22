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
    status: 'active',
    created_at: '2026-04-21T10:00:00Z',
    ended_at: null,
    recording: { active: true, file_count: 2, total_bytes: 1024 },
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
      expect(screen.getAllByText(/tmpl-1/).length).toBeGreaterThanOrEqual(1)
      expect(screen.getAllByText('active').length).toBeGreaterThanOrEqual(1)
    })
  })

  it('renders session info card with recording status', async () => {
    render(
      <MemoryRouter initialEntries={['/sessions/session-1']}>
        <Routes>
          <Route path="/sessions/:id" element={<SessionDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByText('Session Info')).toBeInTheDocument()
      expect(screen.getByText(/Active/)).toBeInTheDocument()
      expect(screen.getByText(/2 files/)).toBeInTheDocument()
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

  it('displays ended_at when session has ended', async () => {
    const { getSession } = await import('@/lib/api/client')
    vi.mocked(getSession).mockResolvedValueOnce({
      id: 'session-2',
      name: 'Ended Session',
      template_id: 'tmpl-1',
      status: 'ended',
      created_at: '2026-04-21T10:00:00Z',
      ended_at: '2026-04-21T11:00:00Z',
    })

    render(
      <MemoryRouter initialEntries={['/sessions/session-2']}>
        <Routes>
          <Route path="/sessions/:id" element={<SessionDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByText('Ended Session')).toBeInTheDocument()
      expect(screen.getAllByText('ended').length).toBeGreaterThanOrEqual(1)
      expect(screen.getByText('Ended')).toBeInTheDocument()
    })
  })
})
