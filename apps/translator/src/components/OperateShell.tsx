import type { CSSProperties, ReactNode } from "react";
import { Link } from "react-router-dom";
import { Button, Logo } from "@crosstalk/theme";

export interface OperateShellProps {
  /** Current operator identity (username). */
  username?: string | null;
  /** Short scope line, e.g. "Assigned sessions". */
  scope?: string | null;
  /** Optional back target. When set, shows a Back control. */
  backTo?: string;
  backLabel?: string;
  onLogout?: () => void;
  /** Sticky connection strip or other top-of-main chrome. */
  strip?: ReactNode;
  children: ReactNode;
}

const skipLinkStyle: CSSProperties = {
  position: "absolute",
  width: 1,
  height: 1,
  padding: 0,
  margin: -1,
  overflow: "hidden",
  clip: "rect(0, 0, 0, 0)",
  whiteSpace: "nowrap",
  border: 0,
};

const skipLinkFocusStyle: CSSProperties = {
  position: "fixed",
  left: "var(--house-space-3)",
  top: "var(--house-space-3)",
  zIndex: 100,
  width: "auto",
  height: "auto",
  margin: 0,
  padding: "var(--house-space-2) var(--house-space-3)",
  overflow: "visible",
  clip: "auto",
  whiteSpace: "normal",
  background: "var(--house-bg-surface)",
  color: "var(--house-text-primary)",
  border: "1px solid var(--house-rule-strong)",
  borderRadius: "var(--house-radius-md)",
  textDecoration: "none",
  font: "500 var(--house-type-control) / var(--house-leading-control) var(--house-font-product)",
};

/**
 * Lightweight translator Operate shell — not the admin sidebar.
 * Persistent identity + back/logout, skip link, main landmark.
 */
export function OperateShell({
  username,
  scope,
  backTo,
  backLabel = "Back",
  onLogout,
  strip,
  children,
}: OperateShellProps) {
  return (
    <div
      className="min-h-screen"
      style={{
        background: "var(--house-bg-canvas)",
        color: "var(--house-text-primary)",
        fontFamily: "var(--house-font-product)",
      }}
    >
      <a
        href="#main-content"
        style={skipLinkStyle}
        onFocus={(e) => Object.assign(e.currentTarget.style, skipLinkFocusStyle)}
        onBlur={(e) => Object.assign(e.currentTarget.style, skipLinkStyle)}
      >
        Skip to main content
      </a>

      <header
        style={{
          borderBottom: "1px solid var(--house-rule-subtle)",
          background: "var(--house-bg-surface)",
          position: "sticky",
          top: 0,
          zIndex: 40,
        }}
      >
        <div
          className="mx-auto flex max-w-3xl flex-wrap items-center justify-between gap-3 px-4"
          style={{
            minHeight: "var(--house-control-height)",
            paddingTop: "var(--house-space-3)",
            paddingBottom: "var(--house-space-3)",
          }}
        >
          <div className="flex min-w-0 items-center gap-3">
            {backTo ? (
              <Link
                to={backTo}
                className="inline-flex items-center gap-2 house-type-control"
                style={{
                  color: "var(--house-text-secondary)",
                  textDecoration: "none",
                  minHeight: 44,
                  padding: "0 var(--house-space-2)",
                }}
              >
                ← {backLabel}
              </Link>
            ) : (
              <Logo className="h-8 w-auto shrink-0" />
            )}
            <div className="min-w-0">
              {username ? (
                <p
                  className="truncate house-type-body"
                  style={{ margin: 0, fontWeight: 600 }}
                  data-testid="operate-user"
                >
                  Logged in as {username}
                </p>
              ) : (
                <p className="house-type-section" style={{ margin: 0 }}>
                  Translator
                </p>
              )}
              {scope ? (
                <p
                  className="truncate house-type-meta"
                  style={{ margin: 0, color: "var(--house-text-tertiary)" }}
                  data-testid="operate-scope"
                >
                  {scope}
                </p>
              ) : null}
            </div>
          </div>

          {onLogout ? (
            <Button
              variant="secondary"
              onClick={onLogout}
              data-testid="operate-logout"
              style={{ minWidth: 88, minHeight: 44 }}
            >
              Logout
            </Button>
          ) : null}
        </div>
        {strip ? (
          <div
            style={{
              borderTop: "1px solid var(--house-rule-subtle)",
              background: "var(--house-bg-canvas)",
            }}
          >
            <div className="mx-auto max-w-3xl px-4 py-3">{strip}</div>
          </div>
        ) : null}
      </header>

      <main id="main-content" className="mx-auto max-w-3xl px-4 py-6" tabIndex={-1}>
        {children}
      </main>
    </div>
  );
}
