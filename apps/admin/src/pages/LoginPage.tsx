import { useEffect, useId, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { Button, Logo } from "@crosstalk/theme";
import { useAuth } from "../hooks/useAuth";

export function LoginPage() {
  const { login, isAuthenticated, isAdmin } = useAuth();
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const usernameId = useId();
  const passwordId = useId();
  const errorId = useId();

  useEffect(() => {
    if (isAuthenticated && isAdmin) {
      navigate("/dashboard", { replace: true });
    }
  }, [isAuthenticated, isAdmin, navigate]);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      await login(username, password);
      navigate("/dashboard");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
      requestAnimationFrame(() => {
        document.getElementById(usernameId)?.focus();
      });
    } finally {
      setLoading(false);
    }
  };

  const fieldClass =
    "w-full min-h-[var(--house-control-height)] rounded-[var(--house-radius-md)] border border-[var(--house-rule-strong)] bg-[var(--house-bg-sunken)] px-3 text-sm text-foreground outline-none focus-visible:ring-2 focus-visible:ring-[var(--house-focus)] focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:opacity-55";

  return (
    <div className="flex min-h-screen items-center justify-center bg-background p-4">
      <div className="w-full max-w-sm">
        <section
          aria-labelledby="admin-login-title"
          className="border border-border bg-[var(--house-bg-surface)] p-6 md:p-8"
        >
          <div className="mb-6">
            <Logo className="h-16 w-auto" />
            <p className="house-type-eyebrow mt-4">Access</p>
            <h1 id="admin-login-title" className="house-type-title mt-1">
              Admin sign in
            </h1>
            <p className="house-type-lede mt-2 text-muted-foreground">
              Authenticate with an admin account to operate sessions, ABCs, and
              translators.
            </p>
          </div>

          {error ? (
            <div
              id={errorId}
              role="alert"
              className="mb-4 border border-[var(--house-status-danger)] bg-[var(--house-status-danger-bg)] px-3 py-2 text-sm text-[var(--house-status-danger)]"
            >
              {error}
            </div>
          ) : null}

          <form
            onSubmit={handleSubmit}
            className="space-y-4"
            aria-describedby={error ? errorId : undefined}
          >
            <div className="flex flex-col gap-1">
              <label htmlFor={usernameId} className="house-type-label text-muted-foreground">
                Username
              </label>
              <input
                id={usernameId}
                name="username"
                type="text"
                autoComplete="username"
                autoFocus
                required
                disabled={loading}
                value={username}
                onChange={(e) => setUsername(e.target.value)}
                placeholder="admin"
                className={fieldClass}
              />
            </div>

            <div className="flex flex-col gap-1">
              <label htmlFor={passwordId} className="house-type-label text-muted-foreground">
                Password
              </label>
              <input
                id={passwordId}
                name="password"
                type="password"
                autoComplete="current-password"
                required
                disabled={loading}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
                className={fieldClass}
              />
            </div>

            <Button
              type="submit"
              variant="primary"
              loading={loading}
              disabled={loading}
              style={{ width: "100%" }}
            >
              {loading ? "Signing in..." : "Sign In"}
            </Button>
          </form>
        </section>
      </div>
    </div>
  );
}
