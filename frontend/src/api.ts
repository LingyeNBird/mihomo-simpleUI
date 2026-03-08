import type { AppStatus, ConfigSyncResult, ProxyGroup, RefreshResult, Subscription } from "./types";

async function request<T>(input: string, init?: RequestInit): Promise<T> {
  const response = await fetch(input, {
    headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
    ...init,
  });

  if (!response.ok) {
    const body = (await response.json().catch(() => null)) as { error?: string } | null;
    throw new Error(body?.error || `Request failed: ${response.status}`);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

export const api = {
  health: () => request<{ ok: boolean }>("/api/health"),
  status: () => request<AppStatus>("/api/status"),
  listSubscriptions: () => request<Subscription[]>("/api/subscriptions"),
  createSubscription: (payload: { name: string; url: string; enabled: boolean }) =>
    request<RefreshResult>("/api/subscriptions", { method: "POST", body: JSON.stringify(payload) }),
  updateSubscription: (id: number, payload: { name: string; url: string; enabled: boolean }) =>
    request<Subscription>(`/api/subscriptions/${id}`, { method: "PUT", body: JSON.stringify(payload) }),
  deleteSubscription: (id: number) => request<void>(`/api/subscriptions/${id}`, { method: "DELETE" }),
  refreshSubscription: (id: number) => request<RefreshResult>(`/api/subscriptions/${id}/refresh`, { method: "POST" }),
  syncConfig: () => request<ConfigSyncResult>("/api/config/sync", { method: "POST" }),
  proxyGroups: () => request<ProxyGroup[]>("/api/proxy-groups"),
  selectNode: (groupName: string, nodeName: string) =>
    request<void>(`/api/proxy-groups/${encodeURIComponent(groupName)}/select`, {
      method: "POST",
      body: JSON.stringify({ nodeName }),
    }),
};
