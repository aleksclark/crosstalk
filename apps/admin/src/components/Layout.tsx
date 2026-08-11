import { useCallback, useEffect, useId, useRef, useState } from "react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import { Button, Icon, IconButton, Logo, type IconName } from "@crosstalk/theme";
import { useAuth } from "../hooks/useAuth";
import { cn } from "../lib/utils";

const navItems: { to: string; label: string; icon: IconName }[] = [
  { to: "/dashboard", label: "Dashboard", icon: "dashboard" },
  { to: "/sessions", label: "Sessions", icon: "sessions" },
  { to: "/abcs", label: "ABCs", icon: "audio" },
  { to: "/translators", label: "Translators", icon: "translators" },
  { to: "/debug", label: "Debug", icon: "debug" },
];

const FOCUSABLE =
  'a[href], button:not([disabled]), textarea, input, select, [tabindex]:not([tabindex="-1"])';

export function Layout() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [navOpen, setNavOpen] = useState(false);
  const menuButtonRef = useRef<HTMLButtonElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  const titleId = useId();

  const closeNav = useCallback(() => {
    setNavOpen(false);
    // Restore focus to the opener on the next frame so the dialog unmounts first.
    requestAnimationFrame(() => menuButtonRef.current?.focus());
  }, []);

  const handleLogout = () => {
    logout();
    navigate("/login");
  };

  // Close mobile nav on route change.
  useEffect(() => {
    setNavOpen(false);
  }, [location.pathname]);

  // Focus trap + Escape while the mobile navigation dialog is open.
  useEffect(() => {
    if (!navOpen) return;
    const dialog = dialogRef.current;
    const focusable = dialog
      ? Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
          (el) => !el.hasAttribute("disabled") && el.tabIndex !== -1,
        )
      : [];
    focusable[0]?.focus();

    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        closeNav();
        return;
      }
      if (e.key !== "Tab" || focusable.length === 0) return;
      const first = focusable[0]!;
      const last = focusable[focusable.length - 1]!;
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = prevOverflow;
    };
  }, [navOpen, closeNav]);

  const navLinkClass = ({ isActive }: { isActive: boolean }) =>
    cn(
      "flex items-center gap-3 rounded-[var(--house-radius-md)] px-3 py-2 text-sm font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-[var(--house-focus)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--house-bg-surface)]",
      isActive
        ? "bg-[var(--house-selected-bg)] text-[var(--house-text-primary)] border-l-2 border-[var(--house-accent)] pl-[10px]"
        : "text-[var(--house-text-secondary)] hover:bg-[var(--house-bg-raised)] hover:text-[var(--house-text-primary)] border-l-2 border-transparent",
    );

  const renderNav = (onNavigate?: () => void) => (
    <>
      <div className="flex items-center gap-3 border-b border-border px-5 py-3">
        <Logo className="h-9 w-auto" />
        <span className="house-type-meta text-muted-foreground">Admin</span>
      </div>

      <nav
        className="flex-1 space-y-1 overflow-y-auto px-3 py-4"
        aria-label="Primary"
      >
        {navItems.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            onClick={onNavigate}
            className={navLinkClass}
          >
            <Icon name={item.icon} size="default" aria-hidden />
            <span>{item.label}</span>
          </NavLink>
        ))}
      </nav>

      <div className="border-t border-border px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0 text-sm">
            <p className="truncate font-medium text-foreground">
              {user?.username ?? "Admin"}
            </p>
            <p className="house-type-meta truncate text-muted-foreground">
              {user?.role ?? "admin"}
            </p>
          </div>
          <Button variant="ghost" size="sm" onClick={handleLogout}>
            Logout
          </Button>
        </div>
      </div>
    </>
  );

  return (
    <div className="flex h-screen overflow-hidden bg-background text-foreground">
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:absolute focus:left-4 focus:top-4 focus:z-[60] focus:rounded-[var(--house-radius-md)] focus:bg-[var(--house-accent)] focus:px-3 focus:py-2 focus:text-[var(--house-accent-ink)] focus:outline-none"
      >
        Skip to main content
      </a>

      {/* Desktop / tablet persistent rail: 218px desktop, 186px tablet */}
      <aside
        className="relative hidden h-full shrink-0 flex-col border-r border-border bg-[var(--house-bg-surface)] md:flex md:w-[186px] xl:w-[218px]"
        aria-label="Admin navigation"
      >
        {renderNav()}
      </aside>

      {/* Mobile navigation dialog */}
      {navOpen ? (
        <div
          className="fixed inset-0 z-50 md:hidden"
          role="presentation"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) closeNav();
          }}
        >
          <div className="absolute inset-0 bg-black/55" aria-hidden />
          <div
            ref={dialogRef}
            role="dialog"
            aria-modal="true"
            aria-labelledby={titleId}
            className="absolute inset-y-0 left-0 flex w-[min(280px,88vw)] flex-col bg-[var(--house-bg-surface)] shadow-none outline-none"
          >
            <div className="flex items-center justify-between border-b border-border px-4 py-3">
              <h2 id={titleId} className="house-type-section m-0">
                Navigation
              </h2>
              <IconButton icon="close" label="Close navigation" onClick={closeNav} />
            </div>
            <div className="flex min-h-0 flex-1 flex-col">{renderNav(closeNav)}</div>
          </div>
        </div>
      ) : null}

      <div className="flex min-w-0 flex-1 flex-col overflow-hidden">
        <header className="flex items-center justify-between border-b border-border px-4 py-3 md:hidden">
          <button
            ref={menuButtonRef}
            type="button"
            aria-label="Open navigation"
            aria-expanded={navOpen}
            aria-haspopup="dialog"
            onClick={() => setNavOpen(true)}
            className="inline-flex h-11 w-11 items-center justify-center rounded-[var(--house-radius-md)] text-foreground outline-none focus-visible:ring-2 focus-visible:ring-[var(--house-focus)] focus-visible:ring-offset-2 focus-visible:ring-offset-background"
          >
            <Icon name="menu" size="default" aria-hidden />
          </button>
          <div className="flex items-center gap-2">
            <Logo className="h-8 w-auto" />
            <span className="house-type-meta text-muted-foreground">Admin</span>
          </div>
          <div className="w-10" aria-hidden />
        </header>

        <main
          id="main-content"
          tabIndex={-1}
          className="flex-1 overflow-y-auto p-4 md:p-6"
        >
          <Outlet />
        </main>
      </div>
    </div>
  );
}
