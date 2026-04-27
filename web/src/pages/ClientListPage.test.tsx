// @ts-nocheck
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { ClientListPage } from './ClientListPage'
import type { Client, ConnectedClient } from '@/lib/api/types'

const mockNavigate = vi.fn()
const mockDeleteClient = vi.fn().mockResolvedValue(undefined)

const mockClients: Client[] = [
  {
    id: 'client-1',
    name: 'K2B Booth 1',
    owner_id: 'user-1',
    source_name: 'alsa_input.platform-snd_aloop.0.analog-stereo',
    sink_name: 'alsa_output.platform-snd_aloop.0.analog-stereo',
    created_at: '2026-04-21T09:00:00Z',
    updated_at: '2026-04-21T09:00:00Z',
  },
  {
    id: 'client-2',
    name: 'Studio Laptop',
    owner_id: 'user-1',
    source_name: '',
    sink_name: '',
    created_at: '2026-04-22T10:00:00Z',
    updated_at: '2026-04-22T10:00:00Z',
  },
]

const mockConnections: ConnectedClient[] = [
  {
    id: 'peer-1',
    client_id: 'client-1',
    client_name: 'K2B Booth 1',
    session_id: '',
    role: '',
    connected_at: '2026-04-22T15:00:00Z',
    sources: [],
    sinks: [],
    codecs: [],
  },
]

vi.mock('@/lib/api/client', () => ({
  getClients: () => Promise.resolve(mockClients),
  getConnections: () => Promise.resolve(mockConnections),
  deleteClient: (...args: unknown[]) => mockDeleteClient(...args),
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

describe('ClientListPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
  })

  it('renders client list with online status', async () => {
    render(
      <MemoryRouter>
        <ClientListPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      const rows = screen.getAllByTestId('client-row')
      expect(rows).toHaveLength(2)
      expect(screen.getAllByText('K2B Booth 1').length).toBeGreaterThanOrEqual(1)
      expect(screen.getByText('Studio Laptop')).toBeInTheDocument()
      expect(screen.getByText('Online')).toBeInTheDocument()
      expect(screen.getByText('Offline')).toBeInTheDocument()
    })
  })

  it('renders live connections table', async () => {
    render(
      <MemoryRouter>
        <ClientListPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      const connRows = screen.getAllByTestId('connection-row')
      expect(connRows).toHaveLength(1)
    })
  })

  it('navigates to create on button click', async () => {
    render(
      <MemoryRouter>
        <ClientListPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getAllByText('K2B Booth 1').length).toBeGreaterThanOrEqual(1)
    })

    fireEvent.click(screen.getByTestId('create-client-button'))
    expect(mockNavigate).toHaveBeenCalledWith('/clients/new')
  })

  it('deletes a client', async () => {
    render(
      <MemoryRouter>
        <ClientListPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getAllByTestId('delete-client-button')).toHaveLength(2)
    })

    fireEvent.click(screen.getAllByTestId('delete-client-button')[0])

    await waitFor(() => {
      expect(mockDeleteClient).toHaveBeenCalledWith('client-1')
    })
  })
})
