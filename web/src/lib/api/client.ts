import type { AgentEvent } from '../types';

declare global {
  interface Window {
    InkweaveNative?: {
      setTheme?: (theme: string) => void;
      shareFile?: (url: string, filename: string) => void;
      isAndroid?: () => boolean;
      getNovelFolder?: () => string;
      pickNovelFolder?: () => void;
      syncNovelFolderNow?: () => void;
      clearNovelFolder?: () => void;
    };
    __inkweaveOnAndroidFolderChanged?: (payloadJson: string) => void;
  }
}

let cachedToken = '';

export function initAuthToken(): string {
  if (cachedToken) return cachedToken;
  const params = new URLSearchParams(window.location.search);
  const qTok = params.get('token');
  if (qTok) {
    cachedToken = qTok;
    try {
      sessionStorage.setItem('inkweave_token', qTok);
      const cleanUrl = window.location.pathname + window.location.hash;
      window.history.replaceState({}, '', cleanUrl);
    } catch {
      // ignore
    }
    return cachedToken;
  }
  try {
    cachedToken = sessionStorage.getItem('inkweave_token') || '';
  } catch {
    cachedToken = '';
  }
  return cachedToken;
}

export function getAuthToken(): string {
  return cachedToken || initAuthToken();
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = getAuthToken();
  const headers = new Headers(init.headers || {});
  if (!headers.has('Content-Type') && init.body && typeof init.body === 'string') {
    headers.set('Content-Type', 'application/json');
  }
  if (token) {
    headers.set('Authorization', `Bearer ${token}`);
  }
  const resp = await fetch(path, {
    ...init,
    headers,
    credentials: 'same-origin',
  });
  if (!resp.ok) {
    let msg = `HTTP ${resp.status}`;
    try {
      const errObj = await resp.json();
      if (errObj?.error) msg = errObj.error;
    } catch {
      // ignore
    }
    throw new Error(msg);
  }
  const ct = resp.headers.get('Content-Type') || '';
  if (ct.includes('application/json')) {
    return (await resp.json()) as T;
  }
  return (await resp.text()) as unknown as T;
}

export async function streamAgentRun(
  projectId: string,
  payload: Record<string, unknown>,
  onEvent: (ev: AgentEvent) => void,
  signal?: AbortSignal,
): Promise<void> {
  const token = getAuthToken();
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  const resp = await fetch(`/api/projects/${projectId}/agent/run`, {
    method: 'POST',
    headers,
    body: JSON.stringify(payload),
    signal,
    credentials: 'same-origin',
  });
  if (!resp.ok || !resp.body) {
    throw new Error(`Agent run failed: HTTP ${resp.status}`);
  }

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buf = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buf += decoder.decode(value, { stream: true });
    const parts = buf.split('\n\n');
    buf = parts.pop() || '';
    for (const part of parts) {
      const lines = part.split('\n');
      for (const line of lines) {
        if (line.startsWith('data:')) {
          const raw = line.slice(5).trim();
          if (!raw) continue;
          try {
            onEvent(JSON.parse(raw) as AgentEvent);
          } catch {
            // ignore partial chunk
          }
        }
      }
    }
  }
}

export function exportUrl(projectId: string, kind: 'markdown' | 'epub'): string {
  const token = getAuthToken();
  const q = token ? `?token=${encodeURIComponent(token)}` : '';
  return `/api/projects/${projectId}/export/${kind}${q}`;
}
