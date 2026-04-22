import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { TemplateListPage } from './TemplateListPage'
import type { SessionTemplate } from '@/lib/api/types'

const mockNavigate = vi.fn()
const mockDeleteTemplate = vi.fn().mockResolvedValue(undefined)

const mockTemplates: SessionTemplate[] = [
  {
    id: 'tmpl-1',
    name: 'Translation',
    is_default: true,
    roles: [
      { name: 'translator', multi_client: false },
      { name: 'studio', multi_client: false },
    ],
    mappings: [
      { from_role: 'translator', from_channel: 'mic', to_role: 'studio', to_channel: 'output', to_type: 'role' },
    ],
    created_at: '2026-04-21T09:00:00Z',
    updated_at: '2026-04-21T09:00:00Z',
  },
  {
    id: 'tmpl-2',
    name: 'Interview',
    is_default: false,
    roles: [{ name: 'host', multi_client: false }],
    mappings: [],
    created_at: '2026-04-21T09:00:00Z',
    updated_at: '2026-04-21T09:00:00Z',
  },
]

vi.mock('@/lib/api/client', () => ({
  getTemplates: () => Promise.resolve(mockTemplates),
  deleteTemplate: (...args: unknown[]) => mockDeleteTemplate(...args),
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

describe('TemplateListPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.spyOn(window, 'confirm').mockReturnValue(true)
  })

  it('renders template list from mock data', async () => {
    render(
      <MemoryRouter>
        <TemplateListPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      const rows = screen.getAllByTestId('template-row')
      expect(rows).toHaveLength(2)
      expect(screen.getByText('Translation')).toBeInTheDocument()
      expect(screen.getByText('Interview')).toBeInTheDocument()
    })
  })

  it('shows default badge for default template', async () => {
    render(
      <MemoryRouter>
        <TemplateListPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      // "Default" appears as both a table header and a badge. Find the badge specifically.
      const defaults = screen.getAllByText('Default')
      // Should have at least 2: the <th> header and the badge
      expect(defaults.length).toBeGreaterThanOrEqual(2)
    })
  })

  it('navigates to create template on button click', async () => {
    render(
      <MemoryRouter>
        <TemplateListPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      fireEvent.click(screen.getByTestId('create-template-button'))
      expect(mockNavigate).toHaveBeenCalledWith('/templates/new')
    })
  })

  it('deletes template on confirmation', async () => {
    render(
      <MemoryRouter>
        <TemplateListPage />
      </MemoryRouter>,
    )

    await waitFor(() => {
      expect(screen.getByText('Translation')).toBeInTheDocument()
    })

    // There are 2 delete buttons
    const deleteButtons = screen.getAllByText('Delete')
    fireEvent.click(deleteButtons[0])

    await waitFor(() => {
      expect(mockDeleteTemplate).toHaveBeenCalledWith('tmpl-1')
    })
  })
})
