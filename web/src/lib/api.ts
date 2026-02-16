const BASE = "";

export class SuperpositionOfflineError extends Error {
  constructor() {
    super("Superposition is offline");
    this.name = "SuperpositionOfflineError";
  }
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${BASE}${path}`, {
      headers: { "Content-Type": "application/json" },
      ...options,
    });
  } catch {
    throw new SuperpositionOfflineError();
  }
  if (res.status === 401) {
    window.location.href = "/auth/login";
    throw new Error("Unauthorized");
  }
  if (res.status === 502) {
    throw new SuperpositionOfflineError();
  }
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  let data: any;
  try {
    data = JSON.parse(text);
  } catch {
    throw new SuperpositionOfflineError();
  }
  if (!res.ok) throw new Error(data.error || "Request failed");
  return data;
}

export const api = {
  // Health
  health: () => request<any>("/api/health"),

  // Gateway health (bypasses tunnel, always reachable when gateway is up)
  gatewayHealth: () =>
    request<{ status: string; gateway: boolean; connected: boolean }>(
      "/gateway/health",
    ),

  // Settings
  getSettings: () => request<any[]>("/api/settings"),
  getSetting: (key: string) => request<any>(`/api/settings/${key}`),
  putSetting: (key: string, value: string) =>
    request<any>(`/api/settings/${key}`, {
      method: "PUT",
      body: JSON.stringify({ value }),
    }),
  deleteSetting: (key: string) =>
    request<void>(`/api/settings/${key}`, { method: "DELETE" }),

  // GitHub repos
  getGitHubRepos: (query?: string, refresh?: boolean) => {
    const params = new URLSearchParams();
    if (query) params.set("q", query);
    if (refresh) params.set("refresh", "true");
    const qs = params.toString();
    return request<any[]>(`/api/github/repos${qs ? `?${qs}` : ""}`);
  },

  // Repos
  getRepos: () => request<any[]>("/api/repos"),
  addRepo: (githubUrl: string) =>
    request<any>("/api/repos", {
      method: "POST",
      body: JSON.stringify({ github_url: githubUrl }),
    }),
  addLocalRepo: (path: string) =>
    request<any>("/api/repos", {
      method: "POST",
      body: JSON.stringify({ local_path: path }),
    }),
  deleteRepo: (id: number) =>
    request<void>(`/api/repos/${id}`, { method: "DELETE" }),
  syncRepo: (id: number) =>
    request<any>(`/api/repos/${id}/sync`, { method: "POST" }),
  getRepoBranches: (id: number) =>
    request<string[]>(`/api/repos/${id}/branches`),

  // Sessions
  getSessions: () => request<any[]>("/api/sessions"),
  createSession: (
    repoId: number,
    sourceBranch: string,
    newBranch: string,
    cliType: string,
  ) =>
    request<any>("/api/sessions", {
      method: "POST",
      body: JSON.stringify({
        repo_id: repoId,
        source_branch: sourceBranch,
        new_branch: newBranch,
        cli_type: cliType,
      }),
    }),
  deleteSession: (id: string, deleteLocal = true) =>
    request<void>(`/api/sessions/${id}?delete_local=${deleteLocal}`, {
      method: "DELETE",
    }),
};
