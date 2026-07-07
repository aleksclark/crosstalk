import {
  createContext,
  useContext,
  useState,
  useCallback,
  useEffect,
  useRef,
  type ReactNode,
} from "react";
import type { components } from "@crosstalk/api-client";
import { getApiClient, getRawApiClient } from "../lib/api";
import { onUnauthorized } from "../lib/errorBus";

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

// Refresh this many milliseconds before the access token's exp so a valid
// token is always in hand before requests are made.
const REFRESH_SKEW_MS = 60_000;

interface JwtClaims {
  sub?: string;
  role?: string;
  exp?: number;
}

// decodeJwt reads the (unverified) payload of a JWT. The server verifies the
// signature; the client only needs the role/subject/exp to drive UI state.
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

// userFromToken derives the UI user from a JWT plus a known username (login
// and refresh responses carry only tokens).
function userFromToken(token: string, username: string): User | null {
  const claims = decodeJwt(token);
  if (!claims) return null;
  return {
    id: claims.sub ?? "",
    username,
    role: (claims.role as User["role"]) ?? "translator",
    created_at: "",
  } as User;
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>(loadAuthState);

  // Latest state for use inside stable callbacks (refresh/logout) without
  // rebuilding them on every token rotation.
  const stateRef = useRef(state);
  stateRef.current = state;

  // Dedupes concurrent refreshes (e.g. several requests 401 at once).
  const refreshInFlight = useRef<Promise<boolean> | null>(null);

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
    const user = userFromToken(token, username);

    setState({
      user,
      token,
      refreshToken: data.refresh_token,
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

  // refresh exchanges the stored refresh token for a fresh access/refresh pair.
  // Returns true on success. Uses the raw client so a failed refresh does not
  // itself re-trigger the global 401 handler.
  const refresh = useCallback(async (): Promise<boolean> => {
    if (refreshInFlight.current) return refreshInFlight.current;
    const refreshToken = stateRef.current.refreshToken;
    if (!refreshToken) return false;

    const run = (async () => {
      const client = getRawApiClient();
      const { data, error } = await client.POST("/api/auth/refresh", {
        body: { refresh_token: refreshToken },
      });
      if (error || !data) return false;
      const username = stateRef.current.user?.username ?? "";
      const user = userFromToken(data.access_token, username);
      setState({
        user,
        token: data.access_token,
        refreshToken: data.refresh_token,
        isAuthenticated: true,
        isAdmin: user?.role === "admin",
      });
      return true;
    })();

    refreshInFlight.current = run;
    try {
      return await run;
    } finally {
      refreshInFlight.current = null;
    }
  }, []);

  // Proactively refresh shortly before the access token expires so requests
  // never race against expiry.
  useEffect(() => {
    if (!state.token) return;
    const claims = decodeJwt(state.token);
    if (!claims?.exp) return;
    const msUntilRefresh = claims.exp * 1000 - Date.now() - REFRESH_SKEW_MS;
    const timer = setTimeout(
      () => {
        void refresh();
      },
      Math.max(0, msUntilRefresh),
    );
    return () => clearTimeout(timer);
  }, [state.token, refresh]);

  // On a 401 from any API call, try one refresh; only log out (→ /login via
  // route guards) if that fails.
  useEffect(
    () =>
      onUnauthorized(() => {
        void refresh().then((ok) => {
          if (!ok) logout();
        });
      }),
    [refresh, logout],
  );

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
