import createClient from "openapi-fetch";
import type { paths } from "./types.js";

export type { paths, components } from "./types.js";

export interface ClientOptions {
  baseUrl: string;
  token?: string;
}

export function createApiClient(options: ClientOptions) {
  const client = createClient<paths>({
    baseUrl: options.baseUrl,
    headers: options.token
      ? { Authorization: `Bearer ${options.token}` }
      : undefined,
  });

  return client;
}

export { createClient };
