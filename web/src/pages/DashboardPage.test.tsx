import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { DashboardPage } from './DashboardPage'
import type { Client, Session, SessionTemplate } from '@/lib/api/types'

const mockNavigate = vi.fn()

const mockClients: Client[] = [
  {
    id: 'client-1',
    role: 'translator',
    session: 'session-1',
    sources: ['mic'],
    sinks: ['speakers'],
    codecs: ['opus'],
    status: 'connected',
    connected_at: '2026-04-21T10:00:00Z',
  },
]

const mockSessions: Session[] = [
  {
    id: 'session-1',
    name: 'Test Session',
    template_id: 'tmpl-1',
    template_name: 'Translation',
    status: 'active',
    client_count: 1,
    total_roles: 2,
    created_at: '2026-04-21T10:00:00Z',
    ended_at: null,
  },
]

const mockTemplates: SessionTemplate[] = [
  {
    id: 'tmpl-1',
    name: 'Translation',
    is_default: true,
    roles: [
      { name: 'translator', multi_client: false },
      { name: 'studio', multi_client: false },
    ],
    mappings: [],
    created_at: '2026-04-21T09:00:00Z',
    updated_at: '2026-04-21T09:00:00Z',
  },
]

vi.mock('@/lib/api/client', () => ({
  getClients: () => Promise.resolve(mockClients),
  getSessions: () => Promise.resolve(mockSessions),
  getTemplates: () => Promise.resolve(mockTemplates),
  createSession: vi.fn().mockResolvedValue({ id: 'new-session', name: 'Quick Test' }),
}))

vi.mock('@/lib/auth', () => ({
  useAuth: () => ({
    user: { id: '1', username: 'admin', created_at: '' },
    isAuthenticated: true,
    login: vi.fn(),
    logout: vi.fn(),
  }),
  AuthProvider: ({ children }: { children: React.ReactNode }) => children,
}))

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

describe('DashboardPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders active sessions count and connected clients count', async () => {
    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByTestId('active-sessions-count')).toHaveTextContent('1')
      expect(screen.getByTestId('connected-clients-count')).toHaveTextContent('1')
    })
  })

  it('renders client table with mock data', async () => {
    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByTestId('client-row')).toBeInTheDocument()
      expect(screen.getByText('client-1')).toBeInTheDocument()
      expect(screen.getByText('translator')).toBeInTheDocument()
    })
  })

  it('quick test button creates session and navigates', async () => {
    const { createSession } = await import('@/lib/api/client')

    render(
      <MemoryRouter>
        <DashboardPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByTestId('quick-test-button')).not.toBeDisabled()
    })

    fireEvent.click(screen.getByTestId('quick-test-button'))

    await waitFor(() => {
      expect(createSession).toHaveBeenCalledWith(
        expect.objectContaining({ template_id: 'tmpl-1' }),
      )
      expect(mockNavigate).toHaveBeenCalledWith('/sessions/new-session/connect?role=translator')
    })
  })
})
