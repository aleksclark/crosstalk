import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { TemplateEditorPage } from './TemplateEditorPage'

const mockNavigate = vi.fn()

vi.mock('@/lib/api/client', () => ({
  getTemplate: vi.fn().mockResolvedValue({
    id: 'tmpl-1',
    name: 'Translation',
    is_default: false,
    roles: [
      { name: 'translator', multi_client: false },
      { name: 'studio', multi_client: false },
    ],
    mappings: [
      { from_role: 'translator', from_channel: 'mic', to_role: 'studio', to_channel: 'output', to_type: 'role' },
    ],
    created_at: '2026-04-21T09:00:00Z',
    updated_at: '2026-04-21T09:00:00Z',
  }),
  createTemplate: vi.fn().mockResolvedValue({ id: 'new-tmpl' }),
  updateTemplate: vi.fn().mockResolvedValue({ id: 'tmpl-1' }),
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

describe('TemplateEditorPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders new template form', async () => {
    render(
      <MemoryRouter initialEntries={['/templates/new']}>
        <Routes>
          <Route path="/templates/:id" element={<TemplateEditorPage />} />
        </Routes>
      </MemoryRouter>,
    )

    expect(screen.getByText('Create Template')).toBeInTheDocument()
    expect(screen.getByTestId('template-name-input')).toBeInTheDocument()
    expect(screen.getByTestId('add-role-button')).toBeInTheDocument()
    expect(screen.getByTestId('add-mapping-button')).toBeInTheDocument()
  })

  it('loads existing template data', async () => {
    render(
      <MemoryRouter initialEntries={['/templates/tmpl-1']}>
        <Routes>
          <Route path="/templates/:id" element={<TemplateEditorPage />} />
        </Routes>
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByText('Edit Template')).toBeInTheDocument()
      expect(screen.getByDisplayValue('Translation')).toBeInTheDocument()
    })
  })

  it('rejects multi-client role as mapping source', async () => {
    render(
      <MemoryRouter initialEntries={['/templates/new']}>
        <Routes>
          <Route path="/templates/:id" element={<TemplateEditorPage />} />
        </Routes>
      </MemoryRouter>,
    )

    // Set up a role with multi_client = true
    const roleNameInput = screen.getByTestId('role-name-input')
    fireEvent.change(roleNameInput, { target: { value: 'audience' } })
    const multiClientToggle = screen.getByTestId('role-multi-client-toggle')
    fireEvent.click(multiClientToggle)

    // Set template name
    fireEvent.change(screen.getByTestId('template-name-input'), { target: { value: 'Test' } })

    // Set up mapping using this multi-client role as source
    const fromRoleSelect = screen.getByTestId('mapping-from-role')
    fireEvent.change(fromRoleSelect, { target: { value: 'audience' } })
    const fromChannelInput = screen.getByTestId('mapping-from-channel')
    fireEvent.change(fromChannelInput, { target: { value: 'mic' } })

    // Try to save
    fireEvent.click(screen.getByTestId('save-template-button'))

    await waitFor(() => {
      expect(screen.getByTestId('template-errors')).toHaveTextContent(
        'Multi-client role "audience" cannot be a mapping source',
      )
    })
  })
})
