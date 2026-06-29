import { createApiClient, type ClientOptions } from "@crosstalk/api-client";

const BASE_URL = import.meta.env.VITE_API_URL || "";

export function getApiClient(token?: string) {
  const options: ClientOptions = {
    baseUrl: BASE_URL,
    token,
  };
  return createApiClient(options);
}
