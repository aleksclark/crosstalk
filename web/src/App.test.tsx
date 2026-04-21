import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { App } from './App'

// Mock all API calls
vi.mock('@/lib/api/client', () => ({
  login: vi.fn(),
  logout: vi.fn(),
  setOnUnauthorized: vi.fn(),
  getClients: vi.fn().mockResolvedValue([]),
  getSessions: vi.fn().mockResolvedValue([]),
  getTemplates: vi.fn().mockResolvedValue([]),
  getServerStatus: vi.fn().mockResolvedValue({ uptime: 0, active_sessions: 0, connected_clients: 0, version: '0.0.0' }),
}))

describe('App', () => {
  it('renders login page when unauthenticated', () => {
    render(<App />)
    expect(screen.getByText('CrossTalk')).toBeInTheDocument()
    expect(screen.getByLabelText('Username')).toBeInTheDocument()
    expect(screen.getByLabelText('Password')).toBeInTheDocument()
  })
})
