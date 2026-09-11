// Thin fetch wrapper: JSON in/out, CSRF header, 401 → login redirect.
import { push } from 'svelte-spa-router';
import { session } from './session.svelte.js';

export class ApiError extends Error {
  constructor(status, body) {
    super(body?.error?.message || `HTTP ${status}`);
    this.status = status;
    this.code = body?.error?.code || 'error';
    this.fields = body?.error?.fields || {};
  }
}

async function request(method, path, body, { redirectOn401 = true } = {}) {
  const res = await fetch('/api' + path, {
    method,
    headers: {
      'X-Requested-With': 'fetch',
      ...(body !== undefined ? { 'Content-Type': 'application/json' } : {}),
    },
    body: body !== undefined ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  });
  if (res.status === 204) return null;
  let data = null;
  const text = await res.text();
  if (text) {
    try { data = JSON.parse(text); } catch { data = null; }
  }
  if (!res.ok) {
    if (res.status === 401 && redirectOn401) {
      session.clear();
      const here = location.hash.slice(1) || '/';
      if (!here.startsWith('/login')) push('/login?next=' + encodeURIComponent(here));
    }
    if (res.status === 503 && data?.error?.code === 'setup_required') {
      session.setupRequired = true;
      if (!location.hash.startsWith('#/setup')) push('/setup');
    }
    throw new ApiError(res.status, data);
  }
  return data;
}

export const api = {
  get: (p, o) => request('GET', p, undefined, o),
  post: (p, b, o) => request('POST', p, b ?? {}, o),
  put: (p, b, o) => request('PUT', p, b ?? {}, o),
  del: (p, b, o) => request('DELETE', p, b, o),
};

export function qs(params) {
  const u = new URLSearchParams();
  for (const [k, v] of Object.entries(params || {})) {
    if (v === undefined || v === null || v === '') continue;
    u.set(k, Array.isArray(v) ? v.join(',') : String(v));
  }
  const s = u.toString();
  return s ? '?' + s : '';
}
