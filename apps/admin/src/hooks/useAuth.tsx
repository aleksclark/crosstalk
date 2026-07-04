import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  type ReactNode,
} from "react";
import type { components } from "@crosstalk/api-client";
import { getApiClient } from "../lib/api";

type User = components["schemas"]["UserOut"];

interface AuthState {
  user: User | null;
  token: string | null;
  refreshToken: string | null;
  isAuthenticated: boolean;
  isAdmin: boolean;
}

interface AuthContextValue extends AuthState {
  login: (username: string, password: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

const STORAGE_KEY = "crosstalk_auth";

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

function loadAuthState(): AuthState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (raw) {
      const data = JSON.parse(raw);
      return {
        user: data.user || null,
        token: data.token || null,
        refreshToken: data.refreshToken || null,
        isAuthenticated: !!data.token,
        isAdmin: data.user?.role === "admin",
      };
    }
  } catch {
    // ignore
  }
  return {
    user: null,
    token: null,
    refreshToken: null,
    isAuthenticated: false,
    isAdmin: false,
  };
}

function saveAuthState(state: AuthState) {
  localStorage.setItem(
    STORAGE_KEY,
    JSON.stringify({
      user: state.user,
      token: state.token,
      refreshToken: state.refreshToken,
    })
  );
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>(loadAuthState);

  useEffect(() => {
    saveAuthState(state);
  }, [state]);

  const login = useCallback(async (username: string, password: string) => {
    const client = getApiClient();
    const { data, error } = await client.POST("/api/auth/login", {
      body: { username, password },
    });

    if (error || !data) {
      throw new Error(error?.detail || "Login failed");
    }

    const token = data.access_token;
    const refreshToken = data.refresh_token;

    // The login response returns only tokens; derive the user (role) from the
    // JWT so route guards that require an admin role work.
    const claims = decodeJwt(token);
    const user: User | null = claims
      ? ({
          id: claims.sub ?? "",
          username,
          role: (claims.role as User["role"]) ?? "translator",
          created_at: "",
        } as User)
      : null;

    setState({
      user,
      token,
      refreshToken,
      isAuthenticated: true,
      isAdmin: user?.role === "admin",
    });
  }, []);

  const logout = useCallback(() => {
    localStorage.removeItem(STORAGE_KEY);
    setState({
      user: null,
      token: null,
      refreshToken: null,
      isAuthenticated: false,
      isAdmin: false,
    });
  }, []);

  return (
    <AuthContext.Provider value={{ ...state, login, logout }}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return ctx;
}
