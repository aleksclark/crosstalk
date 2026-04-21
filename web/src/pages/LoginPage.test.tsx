import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { LoginPage } from './LoginPage'

// Mock the auth context
const mockLogin = vi.fn()
const mockNavigate = vi.fn()

vi.mock('@/lib/auth', () => ({
  useAuth: () => ({
    login: mockLogin,
    isAuthenticated: false,
    user: null,
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

describe('LoginPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders login form with username and password fields', () => {
    render(
      <MemoryRouter>
        <LoginPage />
      </MemoryRouter>,
    )

    expect(screen.getByLabelText('Username')).toBeInTheDocument()
    expect(screen.getByLabelText('Password')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeInTheDocument()
  })

  it('submits credentials and redirects on success', async () => {
    mockLogin.mockResolvedValueOnce(undefined)

    render(
      <MemoryRouter>
        <LoginPage />
      </MemoryRouter>,
    )

    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'password123' } })
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))

    await waitFor(() => {
      expect(mockLogin).toHaveBeenCalledWith('admin', 'password123')
      expect(mockNavigate).toHaveBeenCalledWith('/dashboard')
    })
  })

  it('displays error on login failure', async () => {
    mockLogin.mockRejectedValueOnce(new Error('Invalid credentials'))

    render(
      <MemoryRouter>
        <LoginPage />
      </MemoryRouter>,
    )

    fireEvent.change(screen.getByLabelText('Username'), { target: { value: 'admin' } })
    fireEvent.change(screen.getByLabelText('Password'), { target: { value: 'wrong' } })
    fireEvent.click(screen.getByRole('button', { name: 'Sign in' }))

    await waitFor(() => {
      expect(screen.getByRole('alert')).toHaveTextContent('Invalid credentials')
    })
  })
})
