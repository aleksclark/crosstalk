import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { SessionListPage } from './SessionListPage'
import type { Session, SessionTemplate } from '@/lib/api/types'

const mockNavigate = vi.fn()

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
  {
    id: 'session-2',
    name: 'Old Session',
    template_id: 'tmpl-1',
    template_name: 'Translation',
    status: 'ended',
    client_count: 0,
    total_roles: 2,
    created_at: '2026-04-20T10:00:00Z',
    ended_at: '2026-04-20T11:00:00Z',
  },
]

const mockTemplates: SessionTemplate[] = [
  {
    id: 'tmpl-1',
    name: 'Translation',
    is_default: true,
    roles: [],
    mappings: [],
    created_at: '',
    updated_at: '',
  },
]

const mockEndSession = vi.fn().mockResolvedValue(undefined)

vi.mock('@/lib/api/client', () => ({
  getSessions: () => Promise.resolve(mockSessions),
  getTemplates: () => Promise.resolve(mockTemplates),
  createSession: vi.fn().mockResolvedValue({ id: 'new-session', name: 'New' }),
  endSession: (...args: unknown[]) => mockEndSession(...args),
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

describe('SessionListPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Mock window.confirm
    vi.spyOn(window, 'confirm').mockReturnValue(true)
  })

  it('renders session list from mock data', async () => {
    render(
      <MemoryRouter>
        <SessionListPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      const rows = screen.getAllByTestId('session-row')
      expect(rows).toHaveLength(2)
      expect(screen.getByText('Test Session')).toBeInTheDocument()
      expect(screen.getByText('Old Session')).toBeInTheDocument()
    })
  })

  it('end session calls DELETE API', async () => {
    render(
      <MemoryRouter>
        <SessionListPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByText('Test Session')).toBeInTheDocument()
    })

    // Click "End" on the active session
    const endButton = screen.getByTestId('end-session-button')
    fireEvent.click(endButton)

    await waitFor(() => {
      expect(mockEndSession).toHaveBeenCalledWith('session-1')
    })
  })
})
