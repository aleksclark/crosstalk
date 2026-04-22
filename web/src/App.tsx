import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import { AuthProvider } from '@/lib/auth'
import { useAuth } from '@/lib/use-auth'
import { Layout } from '@/components/layout'
import { LoginPage } from '@/pages/LoginPage'
import { DashboardPage } from '@/pages/DashboardPage'
import { TemplateListPage } from '@/pages/TemplateListPage'
import { TemplateEditorPage } from '@/pages/TemplateEditorPage'
import { SessionListPage } from '@/pages/SessionListPage'
import { SessionDetailPage } from '@/pages/SessionDetailPage'
import { SessionConnectPage } from '@/pages/SessionConnectPage'

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = useAuth()
  if (!isAuthenticated) {
    return <Navigate to="/login" replace />
  }
  return <Layout>{children}</Layout>
}

function AppRoutes() {
  const { isAuthenticated } = useAuth()

  return (
    <Routes>
      <Route
        path="/login"
        element={isAuthenticated ? <Navigate to="/dashboard" replace /> : <LoginPage />}
      />
      <Route
        path="/dashboard"
        element={<ProtectedRoute><DashboardPage /></ProtectedRoute>}
      />
      <Route
        path="/templates"
        element={<ProtectedRoute><TemplateListPage /></ProtectedRoute>}
      />
      <Route
        path="/templates/:id"
        element={<ProtectedRoute><TemplateEditorPage /></ProtectedRoute>}
      />
      <Route
        path="/sessions"
        element={<ProtectedRoute><SessionListPage /></ProtectedRoute>}
      />
      <Route
        path="/sessions/:id"
        element={<ProtectedRoute><SessionDetailPage /></ProtectedRoute>}
      />
      <Route
        path="/sessions/:id/connect"
        element={<ProtectedRoute><SessionConnectPage /></ProtectedRoute>}
      />
      <Route path="*" element={<Navigate to="/dashboard" replace />} />
    </Routes>
  )
}

export function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <AppRoutes />
      </AuthProvider>
    </BrowserRouter>
  )
}
