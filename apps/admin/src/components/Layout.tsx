import { useState } from "react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";
import { cn } from "../lib/utils";

const navItems = [
  { to: "/dashboard", label: "Dashboard", icon: "📊" },
  { to: "/sessions", label: "Sessions", icon: "🎙️" },
  { to: "/abcs", label: "ABCs", icon: "🔌" },
  { to: "/translators", label: "Translators", icon: "🌐" },
  { to: "/debug", label: "Debug", icon: "🐛" },
];

export function Layout() {
  const { user, logout } = useAuth();
  const navigate = useNavigate();
  const [sidebarOpen, setSidebarOpen] = useState(false);

  const handleLogout = () => {
    logout();
    navigate("/login");
  };

  return (
    <div className="flex h-screen overflow-hidden">
      {/* Mobile sidebar overlay */}
      {sidebarOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/50 lg:hidden"
          onClick={() => setSidebarOpen(false)}
        />
      )}

      {/* Sidebar */}
      <aside
        className={cn(
          "fixed inset-y-0 left-0 z-50 w-64 bg-card border-r border-border transform transition-transform duration-200 lg:relative lg:translate-x-0",
          sidebarOpen ? "translate-x-0" : "-translate-x-full"
        )}
      >
        <div className="flex flex-col h-full">
          {/* Logo / brand */}
          <div className="flex items-center gap-2 px-6 py-4 border-b border-border">
            <span className="text-xl font-bold text-primary">CrossTalk</span>
            <span className="text-xs text-muted-foreground bg-muted px-2 py-0.5 rounded">
              Admin
            </span>
          </div>

          {/* Navigation */}
          <nav className="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
            {navItems.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                onClick={() => setSidebarOpen(false)}
                className={({ isActive }) =>
                  cn(
                    "flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium transition-colors",
                    isActive
                      ? "bg-primary text-primary-foreground"
                      : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                  )
                }
              >
                <span>{item.icon}</span>
                <span>{item.label}</span>
              </NavLink>
            ))}
          </nav>

          {/* User section */}
          <div className="border-t border-border px-4 py-3">
            <div className="flex items-center justify-between">
              <div className="text-sm">
                <p className="font-medium">{user?.username ?? "Admin"}</p>
                <p className="text-muted-foreground text-xs">{user?.role}</p>
              </div>
              <button
                onClick={handleLogout}
                className="text-xs text-muted-foreground hover:text-foreground px-2 py-1 rounded hover:bg-accent"
              >
                Logout
              </button>
            </div>
          </div>
        </div>
      </aside>

      {/* Main content */}
      <div className="flex-1 flex flex-col overflow-hidden">
        {/* Mobile header */}
        <header className="lg:hidden flex items-center justify-between border-b border-border px-4 py-3">
          <button
            onClick={() => setSidebarOpen(true)}
            className="text-foreground p-1"
          >
            <svg
              className="w-6 h-6"
              fill="none"
              stroke="currentColor"
              viewBox="0 0 24 24"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                strokeWidth={2}
                d="M4 6h16M4 12h16M4 18h16"
              />
            </svg>
          </button>
          <span className="text-sm font-bold text-primary">CrossTalk Admin</span>
          <div className="w-6" />
        </header>

        {/* Page content */}
        <main className="flex-1 overflow-y-auto p-6">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
