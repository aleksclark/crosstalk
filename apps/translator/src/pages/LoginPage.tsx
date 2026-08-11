import { useEffect, useState, type FormEvent } from "react";
import { useNavigate } from "react-router-dom";
import { Button, Field, Logo, Status } from "@crosstalk/theme";
import { useAuth } from "../hooks/useAuth";

/**
 * Translator access form — house grammar, preserves crosstalk_translator_auth
 * storage/refresh/redirect via useAuth.
 */
export function LoginPage() {
  const { login, token } = useAuth();
  const navigate = useNavigate();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (token) {
      navigate("/", { replace: true });
    }
  }, [token, navigate]);

  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault();
    setError(null);
    setLoading(true);
    try {
      await login(username, password);
      navigate("/", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Login failed");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div
      className="min-h-screen flex items-center justify-center px-4"
      style={{
        background: "var(--house-bg-canvas)",
        color: "var(--house-text-primary)",
        fontFamily: "var(--house-font-product)",
      }}
    >
      <a
        href="#login-form"
        className="house-visually-hidden"
        style={{
          position: "absolute",
          left: "var(--house-space-3)",
          top: "var(--house-space-3)",
          zIndex: 100,
          padding: "var(--house-space-2) var(--house-space-3)",
          background: "var(--house-bg-surface)",
          border: "1px solid var(--house-rule-strong)",
          borderRadius: "var(--house-radius-md)",
        }}
      >
        Skip to login form
      </a>

      <div className="w-full max-w-sm" style={{ display: "flex", flexDirection: "column", gap: "var(--house-space-6)" }}>
        <div style={{ textAlign: "center" }}>
          <Logo className="mx-auto h-24 w-auto" />
          <p
            className="house-type-eyebrow"
            style={{
              marginTop: "var(--house-space-4)",
              color: "var(--house-text-tertiary)",
              letterSpacing: "0.08em",
              textTransform: "uppercase",
            }}
          >
            Translator
          </p>
          <h1 className="house-type-title" style={{ margin: "var(--house-space-2) 0 0" }}>
            Sign in
          </h1>
          <p className="house-type-lede" style={{ margin: "var(--house-space-2) 0 0", color: "var(--house-text-secondary)" }}>
            Access assigned sessions and live audio controls.
          </p>
        </div>

        <form
          id="login-form"
          onSubmit={(e) => void handleSubmit(e)}
          style={{
            display: "flex",
            flexDirection: "column",
            gap: "var(--house-space-4)",
            borderTop: "1px solid var(--house-rule-subtle)",
            paddingTop: "var(--house-space-5)",
          }}
          noValidate
        >
          {error ? (
            <div role="alert">
              <Status tone="danger">{error}</Status>
            </div>
          ) : null}

          <Field
            id="username"
            label="Username"
            name="username"
            type="text"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
            autoComplete="username"
            autoFocus
            placeholder="Enter username"
          />

          <Field
            id="password"
            label="Password"
            name="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoComplete="current-password"
            placeholder="Enter password"
          />

          <Button type="submit" variant="primary" loading={loading} disabled={loading} style={{ width: "100%" }}>
            {loading ? "Signing in..." : "Sign In"}
          </Button>
        </form>
      </div>
    </div>
  );
}
