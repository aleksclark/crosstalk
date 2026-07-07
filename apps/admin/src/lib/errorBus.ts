// errorBus is a tiny module-level pub/sub so non-React code (the API client
// middleware) can surface errors to React components (toasts) and drive the
// logged-out state on 401s.

type ErrorListener = (message: string) => void;
type UnauthorizedListener = () => void;

const errorListeners = new Set<ErrorListener>();
const unauthorizedListeners = new Set<UnauthorizedListener>();

export function onApiError(fn: ErrorListener): () => void {
  errorListeners.add(fn);
  return () => {
    errorListeners.delete(fn);
  };
}

export function emitApiError(message: string): void {
  errorListeners.forEach((fn) => fn(message));
}

export function onUnauthorized(fn: UnauthorizedListener): () => void {
  unauthorizedListeners.add(fn);
  return () => {
    unauthorizedListeners.delete(fn);
  };
}

export function emitUnauthorized(): void {
  unauthorizedListeners.forEach((fn) => fn());
}
