// api.ts centralises requests so the SPA never forgets the CSRF header on a
// mutating endpoint. apiFetch mirrors fetch() but reads the mc_csrf cookie
// and echoes it back in X-CSRF-Token for POST/PUT/PATCH/DELETE.

const CSRF_COOKIE = 'mc_csrf';
const CSRF_HEADER = 'X-CSRF-Token';

const MUTATING = new Set(['POST', 'PUT', 'PATCH', 'DELETE']);

function readCsrfCookie(): string {
  const parts = document.cookie.split(';');
  for (const raw of parts) {
    const [name, ...rest] = raw.trim().split('=');
    if (name === CSRF_COOKIE) {
      return rest.join('=');
    }
  }
  return '';
}

export async function apiFetch(input: string, init: RequestInit = {}): Promise<Response> {
  const method = (init.method ?? 'GET').toUpperCase();
  const headers = new Headers(init.headers ?? {});
  if (MUTATING.has(method)) {
    const token = readCsrfCookie();
    if (token) headers.set(CSRF_HEADER, token);
  }
  return fetch(input, {
    credentials: 'same-origin',
    ...init,
    headers,
  });
}

export async function apiJSON<T>(
  method: string,
  path: string,
  body?: unknown,
): Promise<T> {
  const init: RequestInit = { method };
  if (body !== undefined) {
    init.body = JSON.stringify(body);
    init.headers = { 'Content-Type': 'application/json' };
  }
  const res = await apiFetch(path, init);
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: `HTTP ${res.status}` }));
    throw new Error((err as { error?: string }).error ?? `HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}
