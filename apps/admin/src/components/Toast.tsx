import { Component, useEffect, useState, type ReactNode } from "react";
import { Button } from "@crosstalk/theme";
import { onApiError, emitApiError } from "../lib/errorBus";

interface Toast {
  id: number;
  message: string;
}

let nextId = 1;

// Toaster listens on the error bus and renders dismissible toasts. Toasts stay
// until the user clicks dismiss.
export function Toaster() {
  const [toasts, setToasts] = useState<Toast[]>([]);

  useEffect(() => {
    return onApiError((message) => {
      setToasts((prev) => [...prev, { id: nextId++, message }]);
    });
  }, []);

  const dismiss = (id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  };

  if (toasts.length === 0) return null;

  return (
    <div className="fixed bottom-4 right-4 z-[100] flex max-w-sm flex-col gap-2">
      {toasts.map((toast) => (
        <div
          key={toast.id}
          role="alert"
          className="flex items-start gap-3 border border-[var(--house-status-danger)] bg-[var(--house-status-danger-bg)] px-4 py-3 text-[var(--house-status-danger)]"
        >
          <span className="flex-1 break-words text-sm">{toast.message}</span>
          <button
            type="button"
            onClick={() => dismiss(toast.id)}
            aria-label="Dismiss"
            className="font-bold leading-none opacity-80 hover:opacity-100"
          >
            ×
          </button>
        </div>
      ))}
    </div>
  );
}

interface ErrorBoundaryState {
  hasError: boolean;
}

// ErrorBoundary catches render-time errors anywhere in the tree and surfaces
// them as a toast, then renders a minimal recovery fallback.
export class ErrorBoundary extends Component<
  { children: ReactNode },
  ErrorBoundaryState
> {
  state: ErrorBoundaryState = { hasError: false };

  static getDerivedStateFromError(): ErrorBoundaryState {
    return { hasError: true };
  }

  componentDidCatch(error: Error) {
    emitApiError(error.message || "An unexpected error occurred");
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="flex h-screen flex-col items-center justify-center gap-4 px-4 text-center">
          <h1 className="house-type-title">Something went wrong</h1>
          <Button
            variant="primary"
            onClick={() => this.setState({ hasError: false })}
          >
            Try again
          </Button>
          <Toaster />
        </div>
      );
    }
    return this.props.children;
  }
}
