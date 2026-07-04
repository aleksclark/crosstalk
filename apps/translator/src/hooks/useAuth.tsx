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

const STORAGE_KEY = "crosstalk_translator_auth";

interface StoredAuth {
  token: string;
  refreshToken: string | null;
  user: User | null;
}

function loadStored(): StoredAuth | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as StoredAuth) : null;
  } catch {
    return null;
  }
}

interface JwtClaims {
  sub?: string;
  role?: string;
}

// decodeJwt reads the (unverified) payload of a JWT. The server verifies the
// signature; the client only needs the role/subject to drive UI state.
function decodeJwt(token: string): JwtClaims | null {
  try {
    const payload = token.split(".")[1];
    if (!payload) return null;
    const json = atob(payload.replace(/-/g, "+").replace(/_/g, "/"));
    return JSON.parse(json) as JwtClaims;
  } catch {
    return null;
  }
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const stored = loadStored();
  const [token, setToken] = useState<string | null>(stored?.token ?? null);
  const [user, setUser] = useState<User | null>(stored?.user ?? null);
  const refreshTokenRef = useRef<string | null>(stored?.refreshToken ?? null);
  const tokenRef = useRef<string | null>(stored?.token ?? null);

  const login = useCallback(async (username: string, password: string) => {
    const client = createApiClient({ baseUrl: window.location.origin });
    const { data, error } = await client.POST("/api/auth/login", {
      body: { username, password },
    });
    if (error || !data) {
      throw new Error(error?.detail ?? "Login failed");
    }
    tokenRef.current = data.access_token;
    refreshTokenRef.current = data.refresh_token;
    setToken(data.access_token);

    // The login response returns only tokens; derive the user (id/role) from
    // the JWT so role-based UI works.
    const claims = decodeJwt(data.access_token);
    const u: User | null = claims
      ? {
          id: claims.sub ?? "",
          username,
          role: (claims.role as User["role"]) ?? "translator",
        }
      : null;
    setUser(u);

    // Persist so the session survives reloads and deep links (e.g. opening a
    // /sessions/:id/connect URL directly).
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        token: data.access_token,
        refreshToken: data.refresh_token,
        user: u,
      } satisfies StoredAuth),
    );
  }, []);

  const logout = useCallback(() => {
    tokenRef.current = null;
    refreshTokenRef.current = null;
    setToken(null);
    setUser(null);
    localStorage.removeItem(STORAGE_KEY);
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
