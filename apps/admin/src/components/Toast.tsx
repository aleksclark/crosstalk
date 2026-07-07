import { Component, useEffect, useState, type ReactNode } from "react";
import { onApiError, emitApiError } from "../lib/errorBus";

interface Toast {
  id: number;
  message: string;
}

let nextId = 1;

// Toaster listens on the error bus and renders dismissible toasts. Toasts stay
// until the user clicks the 'x'.
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
    <div className="fixed bottom-4 right-4 z-[100] flex flex-col gap-2 max-w-sm">
      {toasts.map((toast) => (
        <div
          key={toast.id}
          role="alert"
          className="flex items-start gap-3 bg-destructive text-destructive-foreground border border-destructive rounded-lg px-4 py-3 shadow-lg"
        >
          <span className="flex-1 text-sm break-words">{toast.message}</span>
          <button
            onClick={() => dismiss(toast.id)}
            aria-label="Dismiss"
            className="text-destructive-foreground/80 hover:text-destructive-foreground font-bold leading-none"
          >
            ✕
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
        <div className="flex flex-col items-center justify-center h-screen gap-4 text-center px-4">
          <h1 className="text-xl font-bold">Something went wrong</h1>
          <button
            onClick={() => this.setState({ hasError: false })}
            className="bg-primary text-primary-foreground px-4 py-2 rounded-md text-sm font-medium hover:opacity-90"
          >
            Try again
          </button>
          <Toaster />
        </div>
      );
    }
    return this.props.children;
  }
}
