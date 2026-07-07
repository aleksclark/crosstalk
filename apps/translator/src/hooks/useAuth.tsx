import {
  createContext,
  useContext,
  useState,
  useCallback,
  useRef,
  useEffect,
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

// Refresh this many milliseconds before the access token's exp.
const REFRESH_SKEW_MS = 60_000;

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

function userFromToken(token: string, username: string): User | null {
  const claims = decodeJwt(token);
  if (!claims) return null;
  return {
    id: claims.sub ?? "",
    username,
    role: (claims.role as User["role"]) ?? "translator",
  };
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const stored = loadStored();
  const [token, setToken] = useState<string | null>(stored?.token ?? null);
  const [user, setUser] = useState<User | null>(stored?.user ?? null);
  const refreshTokenRef = useRef<string | null>(stored?.refreshToken ?? null);
  const tokenRef = useRef<string | null>(stored?.token ?? null);
  const userRef = useRef<User | null>(stored?.user ?? null);
  const refreshInFlight = useRef<Promise<boolean> | null>(null);

  const persist = useCallback(() => {
    if (!tokenRef.current) {
      localStorage.removeItem(STORAGE_KEY);
      return;
    }
    localStorage.setItem(
      STORAGE_KEY,
      JSON.stringify({
        token: tokenRef.current,
        refreshToken: refreshTokenRef.current,
        user: userRef.current,
      } satisfies StoredAuth),
    );
  }, []);

  const login = useCallback(
    async (username: string, password: string) => {
      const client = createApiClient({ baseUrl: window.location.origin });
      const { data, error } = await client.POST("/api/auth/login", {
        body: { username, password },
      });
      if (error || !data) {
        throw new Error(error?.detail ?? "Login failed");
      }
      const u = userFromToken(data.access_token, username);
      tokenRef.current = data.access_token;
      refreshTokenRef.current = data.refresh_token;
      userRef.current = u;
      setToken(data.access_token);
      setUser(u);
      persist();
    },
    [persist],
  );

  const logout = useCallback(() => {
    tokenRef.current = null;
    refreshTokenRef.current = null;
    userRef.current = null;
    setToken(null);
    setUser(null);
    localStorage.removeItem(STORAGE_KEY);
  }, []);

  const getToken = useCallback(() => tokenRef.current, []);

  // refresh exchanges the stored refresh token for a new access/refresh pair.
  // Returns true on success. Deduped so concurrent callers share one request.
  const refresh = useCallback((): Promise<boolean> => {
    if (refreshInFlight.current) return refreshInFlight.current;
    const rt = refreshTokenRef.current;
    if (!rt) return Promise.resolve(false);

    const run = (async () => {
      const client = createApiClient({ baseUrl: window.location.origin });
      const { data, error } = await client.POST("/api/auth/refresh", {
        body: { refresh_token: rt },
      });
      if (error || !data) return false;
      const u = userFromToken(data.access_token, userRef.current?.username ?? "");
      tokenRef.current = data.access_token;
      refreshTokenRef.current = data.refresh_token;
      userRef.current = u;
      setToken(data.access_token);
      setUser(u);
      persist();
      return true;
    })();

    refreshInFlight.current = run;
    void run.finally(() => {
      refreshInFlight.current = null;
    });
    return run;
  }, [persist]);

  // Proactively refresh shortly before the access token expires. If a stored
  // token is already (near) expired on load, this fires immediately so a page
  // reload after a long idle re-establishes a valid token instead of 401ing.
  useEffect(() => {
    if (!token) return;
    const claims = decodeJwt(token);
    if (!claims?.exp) return;
    const msUntilRefresh = claims.exp * 1000 - Date.now() - REFRESH_SKEW_MS;
    const timer = setTimeout(
      () => {
        void refresh().then((ok) => {
          if (!ok) logout();
        });
      },
      Math.max(0, msUntilRefresh),
    );
    return () => clearTimeout(timer);
  }, [token, refresh, logout]);

  return (
    <AuthContext.Provider
      value={{
        token,
        refreshToken: refreshTokenRef.current,
        user,
        login,
        logout,
        getToken,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthState {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
