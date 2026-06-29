import { Routes, Route, Navigate } from "react-router-dom";
import { useAuth } from "./hooks/useAuth";
import { LoginPage } from "./pages/LoginPage";
import { SessionListPage } from "./pages/SessionListPage";
import { SessionConnectPage } from "./pages/SessionConnectPage";

function AuthGuard({ children }: { children: React.ReactNode }) {
  const { token } = useAuth();
  if (!token) {
    return <Navigate to="/login" replace />;
  }
  return <>{children}</>;
}

export function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        path="/"
        element={
          <AuthGuard>
            <SessionListPage />
          </AuthGuard>
        }
      />
      <Route
        path="/sessions/:id/connect"
        element={
          <AuthGuard>
            <SessionConnectPage />
          </AuthGuard>
        }
      />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  );
}
