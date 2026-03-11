export interface RequestOptions extends Omit<RequestInit, "body"> {
  body?: BodyInit | Record<string, unknown> | null;
}

export class HttpError extends Error {
  readonly status: number;
  readonly payload: unknown;

  constructor(message: string, status: number, payload: unknown) {
    super(message);
    this.name = "HttpError";
    this.status = status;
    this.payload = payload;
  }
}

const defaultHeaders = {
  Accept: "application/json",
} satisfies HeadersInit;

export class HttpClient {
  constructor(private readonly baseURL = "/api") {}

  async request<T>(path: string, options: RequestOptions = {}): Promise<T> {
    const headers = new Headers(defaultHeaders);

    if (options.headers) {
      new Headers(options.headers).forEach((value, key) => headers.set(key, value));
    }

    const response = await fetch(this.resolveURL(path), {
      ...options,
      headers,
      credentials: "include",
      body: normalizeBody(options.body, headers),
    });

    const payload = await parsePayload(response);

    if (!response.ok) {
      throw new HttpError(response.statusText || "Request failed", response.status, payload);
    }

    return payload as T;
  }

  get<T>(path: string, options?: RequestOptions) {
    return this.request<T>(path, { ...options, method: "GET" });
  }

  post<T>(path: string, body?: RequestOptions["body"], options?: RequestOptions) {
    return this.request<T>(path, { ...options, method: "POST", body });
  }

  private resolveURL(path: string) {
    if (/^https?:\/\//.test(path)) {
      return path;
    }
    return `${this.baseURL}${path.startsWith("/") ? path : `/${path}`}`;
  }
}

function normalizeBody(body: RequestOptions["body"], headers: Headers): BodyInit | null | undefined {
  if (body == null) {
    return body;
  }

  if (body instanceof FormData || body instanceof URLSearchParams || typeof body === "string" || body instanceof Blob) {
    return body;
  }

  headers.set("Content-Type", "application/json");
  return JSON.stringify(body);
}

async function parsePayload(response: Response): Promise<unknown> {
  if (response.status === 204) {
    return null;
  }

  const contentType = response.headers.get("content-type") ?? "";

  if (contentType.includes("application/json")) {
    return response.json();
  }

  return response.text();
}

export const httpClient = new HttpClient(import.meta.env.VITE_API_BASE_URL ?? "/api");
