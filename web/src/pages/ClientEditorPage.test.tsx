// @ts-nocheck
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ClientEditorPage } from './ClientEditorPage'

const mockNavigate = vi.fn()

vi.mock('@/lib/api/client', () => ({
  getClient: vi.fn().mockResolvedValue({
    id: 'client-1',
    name: 'K2B Booth 1',
    owner_id: 'user-1',
    source_name: 'alsa_input.platform-snd_aloop.0.analog-stereo',
    sink_name: 'alsa_output.platform-snd_aloop.0.analog-stereo',
    created_at: '2026-04-21T09:00:00Z',
    updated_at: '2026-04-21T09:00:00Z',
  }),
  createClient: vi.fn().mockResolvedValue({ id: 'new-client' }),
  updateClient: vi.fn().mockResolvedValue({ id: 'client-1' }),
  getTokens: vi.fn().mockResolvedValue([
    { id: 'tok-1', name: 'deploy-token', client_id: 'client-1', created_at: '2026-04-22T10:00:00Z' },
    { id: 'tok-2', name: 'other-token', client_id: 'client-99', created_at: '2026-04-22T10:00:00Z' },
  ]),
  createToken: vi.fn().mockResolvedValue({ id: 'tok-3', name: 'new-token', token: 'ct_newplaintext' }),
  revokeToken: vi.fn().mockResolvedValue(undefined),
  getConnections: vi.fn().mockResolvedValue([
    {
      id: 'peer-1',
      client_id: 'client-1',
      client_name: 'K2B Booth 1',
      session_id: '',
      role: '',
      connected_at: '2026-04-22T15:00:00Z',
      sources: [{ name: 'alsa_input.test', type: 'audio' }],
      sinks: [{ name: 'alsa_output.test', type: 'audio' }],
      codecs: ['opus/48000/2'],
    },
  ]),
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
  return { ...actual, useNavigate: () => mockNavigate }
})

function renderEditor(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path="/clients/:id" element={<ClientEditorPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('ClientEditorPage', () => {
  beforeEach(() => {
    mockNavigate.mockClear()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
  })

  it('renders new client form without connection/token sections', () => {
    renderEditor('/clients/new')
    expect(screen.getByText('Create Client')).toBeInTheDocument()
    expect(screen.getByTestId('client-name-input')).toBeInTheDocument()
    expect(screen.queryByText('Connection Status')).not.toBeInTheDocument()
    expect(screen.queryByText('API Tokens')).not.toBeInTheDocument()
  })

  it('loads existing client with connection status and detected devices', async () => {
    renderEditor('/clients/client-1')
    await waitFor(() => {
      expect(screen.getByText('Edit Client')).toBeInTheDocument()
    }, { timeout: 3000 })
    expect(mockNavigate).not.toHaveBeenCalledWith('/clients')
    expect(screen.getByDisplayValue('K2B Booth 1')).toBeInTheDocument()
    expect(screen.getByTestId('connection-status')).toHaveTextContent('Online')
    expect(screen.getByTestId('connection-detail')).toBeInTheDocument()
    expect(screen.getByTestId('detected-source')).toHaveTextContent('alsa_input.test')
    expect(screen.getByTestId('detected-sink')).toHaveTextContent('alsa_output.test')
  })

  it('shows only tokens associated with this client', async () => {
    renderEditor('/clients/client-1')
    await waitFor(() => {
      const rows = screen.getAllByTestId('client-token-row')
      expect(rows).toHaveLength(1)
      expect(screen.getByText('deploy-token')).toBeInTheDocument()
    })
  })

  it('creates a token scoped to this client', async () => {
    const { createToken } = await import('@/lib/api/client')
    renderEditor('/clients/client-1')

    await waitFor(() => expect(screen.getByText('Edit Client')).toBeInTheDocument())

    fireEvent.click(screen.getByTestId('toggle-create-client-token'))
    await waitFor(() => expect(screen.getByTestId('create-client-token-form')).toBeInTheDocument())

    fireEvent.change(screen.getByTestId('client-token-name-input'), { target: { value: 'new-token' } })
    fireEvent.click(screen.getByTestId('confirm-create-client-token'))

    await waitFor(() => {
      expect(createToken).toHaveBeenCalledWith({ name: 'new-token', client_id: 'client-1' })
      expect(screen.getByTestId('client-token-created-banner')).toBeInTheDocument()
      expect(screen.getByTestId('created-client-token-value')).toHaveTextContent('ct_newplaintext')
    })
  })

  it('revokes a client token', async () => {
    const { revokeToken } = await import('@/lib/api/client')
    renderEditor('/clients/client-1')

    await waitFor(() => expect(screen.getByTestId('revoke-client-token-button')).toBeInTheDocument())
    fireEvent.click(screen.getByTestId('revoke-client-token-button'))

    await waitFor(() => {
      expect(revokeToken).toHaveBeenCalledWith('tok-1')
    })
  })

  it('creates a new client and redirects to edit', async () => {
    const { createClient } = await import('@/lib/api/client')
    renderEditor('/clients/new')

    fireEvent.change(screen.getByTestId('client-name-input'), { target: { value: 'New Booth' } })
    fireEvent.click(screen.getByTestId('save-client-button'))

    await waitFor(() => {
      expect(createClient).toHaveBeenCalledWith(expect.objectContaining({ name: 'New Booth' }))
      expect(mockNavigate).toHaveBeenCalledWith('/clients/new-client')
    })
  })
})
