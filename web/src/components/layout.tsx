import { Link, useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '@/lib/auth'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

const navItems = [
  { label: 'Dashboard', path: '/dashboard' },
  { label: 'Templates', path: '/templates' },
  { label: 'Sessions', path: '/sessions' },
]

export function Layout({ children }: { children: React.ReactNode }) {
  const { user, logout } = useAuth()
  const location = useLocation()
  const navigate = useNavigate()

  const handleLogout = async () => {
    await logout()
    navigate('/login')
  }

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b border-border bg-card">
        <div className="flex h-14 items-center px-6">
          <Link to="/dashboard" className="text-lg font-bold text-foreground mr-8">
            CrossTalk
          </Link>
          <nav className="flex items-center gap-1">
            {navItems.map((item) => (
              <Link
                key={item.path}
                to={item.path}
                className={cn(
                  'px-3 py-2 text-sm rounded-md transition-colors',
                  location.pathname.startsWith(item.path)
                    ? 'bg-accent text-accent-foreground'
                    : 'text-muted-foreground hover:text-foreground hover:bg-accent/50',
                )}
              >
                {item.label}
              </Link>
            ))}
          </nav>
          <div className="ml-auto flex items-center gap-4">
            <span className="text-sm text-muted-foreground">{user?.username}</span>
            <Button variant="ghost" size="sm" onClick={handleLogout} data-testid="logout-button">
              Logout
            </Button>
          </div>
        </div>
      </header>
      <main className="p-6">{children}</main>
    </div>
  )
}
