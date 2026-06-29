import { Routes, Route, Navigate } from "react-router-dom";
import { useAuth } from "./hooks/useAuth";
import { Layout } from "./components/Layout";
import { LoginPage } from "./pages/LoginPage";
import { DashboardPage } from "./pages/DashboardPage";
import { SessionListPage } from "./pages/SessionListPage";
import { SessionDetailPage } from "./pages/SessionDetailPage";
import { ABCListPage } from "./pages/ABCListPage";
import { ABCDetailPage } from "./pages/ABCDetailPage";
import { TranslatorListPage } from "./pages/TranslatorListPage";
import { DebugPage } from "./pages/DebugPage";

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated, isAdmin } = useAuth();

  if (!isAuthenticated || !isAdmin) {
    return <Navigate to="/login" replace />;
  }

  return <>{children}</>;
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />
      <Route
        element={
          <ProtectedRoute>
            <Layout />
          </ProtectedRoute>
        }
      >
        <Route path="/dashboard" element={<DashboardPage />} />
        <Route path="/sessions" element={<SessionListPage />} />
        <Route path="/sessions/:id" element={<SessionDetailPage />} />
        <Route path="/abcs" element={<ABCListPage />} />
        <Route path="/abcs/:id" element={<ABCDetailPage />} />
        <Route path="/translators" element={<TranslatorListPage />} />
        <Route path="/debug" element={<DebugPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/dashboard" replace />} />
    </Routes>
  );
}
