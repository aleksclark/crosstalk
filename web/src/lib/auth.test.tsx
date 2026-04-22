import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { AuthProvider } from './auth'
import { useAuth } from './use-auth'

// Mock API client
vi.mock('@/lib/api/client', () => ({
  login: vi.fn().mockResolvedValue({ token: 'test-token', user: { id: '1', username: 'admin', created_at: '' } }),
  logout: vi.fn().mockResolvedValue(undefined),
  setOnUnauthorized: vi.fn(),
  setAuthToken: vi.fn(),
}))

function TestComponent() {
  const { isAuthenticated, user, login, logout } = useAuth()
  return (
    <div>
      <span data-testid="auth-status">{isAuthenticated ? 'authenticated' : 'unauthenticated'}</span>
      <span data-testid="username">{user?.username ?? 'none'}</span>
      <button onClick={() => login('admin', 'pass')}>Login</button>
      <button onClick={() => logout()}>Logout</button>
    </div>
  )
}

describe('AuthContext', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    sessionStorage.clear()
  })

  it('starts unauthenticated', () => {
    render(
      <MemoryRouter>
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      </MemoryRouter>,
    )

    expect(screen.getByTestId('auth-status')).toHaveTextContent('unauthenticated')
    expect(screen.getByTestId('username')).toHaveTextContent('none')
  })

  it('authenticates after login', async () => {
    render(
      <MemoryRouter>
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      </MemoryRouter>,
    )

    fireEvent.click(screen.getByText('Login'))

    await waitFor(() => {
      expect(screen.getByTestId('auth-status')).toHaveTextContent('authenticated')
      expect(screen.getByTestId('username')).toHaveTextContent('admin')
    })
  })

  it('clears auth state after logout', async () => {
    render(
      <MemoryRouter>
        <AuthProvider>
          <TestComponent />
        </AuthProvider>
      </MemoryRouter>,
    )

    // Login first
    fireEvent.click(screen.getByText('Login'))
    await waitFor(() => {
      expect(screen.getByTestId('auth-status')).toHaveTextContent('authenticated')
    })

    // Then logout
    fireEvent.click(screen.getByText('Logout'))
    await waitFor(() => {
      expect(screen.getByTestId('auth-status')).toHaveTextContent('unauthenticated')
    })
  })

  it('redirects to login on 401 via ProtectedRoute', async () => {
    function ProtectedRoute({ children }: { children: React.ReactNode }) {
      const { isAuthenticated } = useAuth()
      if (!isAuthenticated) {
        return <div data-testid="redirected">Redirected to login</div>
      }
      return <>{children}</>
    }

    render(
      <MemoryRouter initialEntries={['/dashboard']}>
        <AuthProvider>
          <Routes>
            <Route path="/dashboard" element={<ProtectedRoute><div>Dashboard</div></ProtectedRoute>} />
          </Routes>
        </AuthProvider>
      </MemoryRouter>,
    )

    expect(screen.getByTestId('redirected')).toBeInTheDocument()
  })
})
