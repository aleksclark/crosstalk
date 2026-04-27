import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { SettingsPage } from './SettingsPage'
import type { User, ApiToken } from '@/lib/api/types'

const mockUsers: User[] = [
  { id: 'user-1', username: 'admin', created_at: '2026-04-21T09:00:00Z' },
  { id: 'user-2', username: 'operator', created_at: '2026-04-22T10:00:00Z' },
]

const mockTokens: ApiToken[] = [
  { id: 'tok-1', name: 'CI Pipeline', created_at: '2026-04-21T09:00:00Z', last_used_at: null },
  { id: 'tok-2', name: 'K2B Board', created_at: '2026-04-22T10:00:00Z', last_used_at: null },
]

const mockCreateUser = vi.fn().mockResolvedValue({ id: 'user-3', username: 'newuser', created_at: '2026-04-22T12:00:00Z' })
const mockDeleteUser = vi.fn().mockResolvedValue(undefined)
const mockCreateToken = vi.fn().mockResolvedValue({ id: 'tok-3', name: 'New Token', token: 'ct_abc123plaintext' })
const mockRevokeToken = vi.fn().mockResolvedValue(undefined)

vi.mock('@/lib/api/client', () => ({
  getUsers: () => Promise.resolve(mockUsers),
  getTokens: () => Promise.resolve(mockTokens),
  createUser: (...args: unknown[]) => mockCreateUser(...args),
  deleteUser: (...args: unknown[]) => mockDeleteUser(...args),
  createToken: (...args: unknown[]) => mockCreateToken(...args),
  revokeToken: (...args: unknown[]) => mockRevokeToken(...args),
}))

vi.mock('@/lib/use-auth', () => ({
  useAuth: () => ({
    user: { id: '1', username: 'admin', created_at: '' },
    isAuthenticated: true,
    login: vi.fn(),
    logout: vi.fn(),
  }),
}))

describe('SettingsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
  })

  it('renders users and tokens from mock data', async () => {
    render(
      <MemoryRouter>
        <SettingsPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getAllByTestId('user-row')).toHaveLength(2)
      expect(screen.getByText('admin')).toBeInTheDocument()
      expect(screen.getByText('operator')).toBeInTheDocument()

      expect(screen.getAllByTestId('token-row')).toHaveLength(2)
      expect(screen.getByText('CI Pipeline')).toBeInTheDocument()
      expect(screen.getByText('K2B Board')).toBeInTheDocument()
    })
  })

  it('creates a new user', async () => {
    render(
      <MemoryRouter>
        <SettingsPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByText('admin')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByTestId('toggle-create-user'))

    await waitFor(() => {
      expect(screen.getByTestId('create-user-form')).toBeInTheDocument()
    })

    fireEvent.change(screen.getByTestId('new-username-input'), { target: { value: 'newuser' } })
    fireEvent.change(screen.getByTestId('new-password-input'), { target: { value: 'secret123' } })
    fireEvent.click(screen.getByTestId('confirm-create-user'))

    await waitFor(() => {
      expect(mockCreateUser).toHaveBeenCalledWith({ username: 'newuser', password: 'secret123' })
    })
  })

  it('deletes a user', async () => {
    render(
      <MemoryRouter>
        <SettingsPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getAllByTestId('delete-user-button')).toHaveLength(2)
    })

    fireEvent.click(screen.getAllByTestId('delete-user-button')[0])

    await waitFor(() => {
      expect(mockDeleteUser).toHaveBeenCalledWith('user-1')
    })
  })

  it('creates a token and shows the plaintext', async () => {
    render(
      <MemoryRouter>
        <SettingsPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByText('CI Pipeline')).toBeInTheDocument()
    })

    fireEvent.click(screen.getByTestId('toggle-create-token'))

    await waitFor(() => {
      expect(screen.getByTestId('create-token-form')).toBeInTheDocument()
    })

    fireEvent.change(screen.getByTestId('new-token-name-input'), { target: { value: 'New Token' } })
    fireEvent.click(screen.getByTestId('confirm-create-token'))

    await waitFor(() => {
      expect(mockCreateToken).toHaveBeenCalledWith({ name: 'New Token' })
      expect(screen.getByTestId('token-created-banner')).toBeInTheDocument()
      expect(screen.getByTestId('created-token-value')).toHaveTextContent('ct_abc123plaintext')
    })
  })

  it('revokes a token', async () => {
    render(
      <MemoryRouter>
        <SettingsPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getAllByTestId('revoke-token-button')).toHaveLength(2)
    })

    fireEvent.click(screen.getAllByTestId('revoke-token-button')[0])

    await waitFor(() => {
      expect(mockRevokeToken).toHaveBeenCalledWith('tok-1')
    })
  })
})
