import { createApiClient, type ClientOptions } from "@crosstalk/api-client";
import { emitApiError, emitUnauthorized } from "./errorBus";

const BASE_URL = import.meta.env.VITE_API_URL || "";

async function extractErrorMessage(response: Response): Promise<string> {
  try {
    const body = await response.clone().json();
    if (body && typeof body === "object" && typeof body.detail === "string") {
      return body.detail;
    }
  } catch {
    // body was not JSON; fall through to status text
  }
  return `${response.status} ${response.statusText}`.trim();
}

export function getApiClient(token?: string) {
  const options: ClientOptions = {
    baseUrl: BASE_URL,
    token,
  };
  const client = createApiClient(options);

  client.use({
    async onResponse({ response }) {
      if (!response.ok) {
        if (response.status === 401) {
          emitUnauthorized();
        }
        emitApiError(await extractErrorMessage(response));
      }
      return response;
    },
    onError({ error }) {
      emitApiError(error instanceof Error ? error.message : String(error));
    },
  });

  return client;
}

// getRawApiClient returns a client without the global error/401 interceptor.
// Used for auth endpoints (login/refresh) where a 401 is handled locally and
// must not trigger the global logout-on-401 flow.
export function getRawApiClient(token?: string) {
  return createApiClient({ baseUrl: BASE_URL, token });
}
