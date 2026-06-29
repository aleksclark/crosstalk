import {
  createContext,
  useContext,
  useState,
  useCallback,
  useRef,
  type ReactNode,
} from "react";
import { createApiClient } from "@crosstalk/api-client";

interface User {
  id: string;
  username: string;
  role: "admin" | "translator";
}

interface AuthState {
  token: string | null;
  refreshToken: string | null;
  user: User | null;
  login: (username: string, password: string) => Promise<void>;
  logout: () => void;
  getToken: () => string | null;
}

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [token, setToken] = useState<string | null>(null);
  const [user, setUser] = useState<User | null>(null);
  const refreshTokenRef = useRef<string | null>(null);
  const tokenRef = useRef<string | null>(null);

  const login = useCallback(async (username: string, password: string) => {
    const client = createApiClient({ baseUrl: window.location.origin });
    const { data, error } = await client.POST("/api/auth/login", {
      body: { username, password },
    });
    if (error || !data) {
      throw new Error(
        error?.error?.message ?? "Login failed",
      );
    }
    tokenRef.current = data.access_token;
    refreshTokenRef.current = data.refresh_token;
    setToken(data.access_token);
    setUser(data.user ?? null);
  }, []);

  const logout = useCallback(() => {
    tokenRef.current = null;
    refreshTokenRef.current = null;
    setToken(null);
    setUser(null);
  }, []);

  const getToken = useCallback(() => {
    return tokenRef.current;
  }, []);

  return (
    <AuthContext.Provider value={{ token, refreshToken: refreshTokenRef.current, user, login, logout, getToken }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
