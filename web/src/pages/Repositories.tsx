import { useEffect, useState, useCallback, useRef } from "react";
import { api } from "../lib/api";

interface LocalRepo {
  id: number;
  github_url: string;
  owner: string;
  name: string;
  clone_status: string;
  default_branch: string;
  last_synced: string | null;
  repo_type: string;
  source_path: string | null;
}

interface GitHubRepo {
  full_name: string;
  html_url: string;
  clone_url: string;
  owner_login: string;
  name: string;
  private: boolean;
  default_branch: string;
  description: string;
}

export default function Repositories() {
  const [localRepos, setLocalRepos] = useState<LocalRepo[]>([]);
  const [ghRepos, setGhRepos] = useState<GitHubRepo[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [hasLoaded, setHasLoaded] = useState(false);
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined);
  const [localPath, setLocalPath] = useState("");
  const [localError, setLocalError] = useState("");

  const loadLocal = useCallback(() => {
    api.getRepos().then(setLocalRepos).catch(console.error);
  }, []);

  useEffect(() => {
    loadLocal();
    const interval = setInterval(loadLocal, 3000);
    return () => clearInterval(interval);
  }, [loadLocal]);

  const searchGitHub = useCallback(async (query: string, refresh?: boolean) => {
    setLoading(true);
    setError("");
    try {
      const repos = await api.getGitHubRepos(query || undefined, refresh);
      setGhRepos(repos || []);
      setHasLoaded(true);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  }, []);

  // Load all repos on mount
  useEffect(() => {
    searchGitHub("");
  }, [searchGitHub]);

  // Debounced search when typing
  useEffect(() => {
    if (!hasLoaded) return;
    clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      searchGitHub(search);
    }, 300);
    return () => clearTimeout(debounceRef.current);
  }, [search, hasLoaded, searchGitHub]);

  const addRepo = async (url: string) => {
    try {
      await api.addRepo(url);
      loadLocal();
    } catch (e: any) {
      setError(e.message);
    }
  };

  const addLocalFolder = async () => {
    const trimmed = localPath.trim();
    if (!trimmed) return;
    setLocalError("");
    try {
      await api.addLocalRepo(trimmed);
      setLocalPath("");
      loadLocal();
    } catch (e: any) {
      setLocalError(e.message);
    }
  };

  const removeRepo = async (id: number) => {
    await api.deleteRepo(id);
    loadLocal();
  };

  const syncRepo = async (id: number) => {
    await api.syncRepo(id);
    loadLocal();
  };

  const localFullNames = new Set(localRepos.map((r) => `${r.owner}/${r.name}`));
  const filteredGh = ghRepos.filter((r) => !localFullNames.has(r.full_name));

  return (
    <div className="w-full max-w-4xl p-4 sm:p-6 lg:p-8">
      <h2 className="text-xl sm:text-2xl font-bold mb-1">Repositories</h2>
      <p className="text-sm sm:text-base text-zinc-400 mb-6 sm:mb-8">
        Add GitHub repos or local folders to start coding sessions.
      </p>

      {error && (
        <div className="mb-4 p-3 bg-red-900/30 border border-red-800 rounded-md text-sm text-red-300">
          {error}
        </div>
      )}

      {/* Local repos */}
      <div className="mb-6 sm:mb-8">
        <h3 className="text-sm font-medium text-zinc-400 uppercase tracking-wider mb-3">
          Local Repositories
        </h3>
        {localRepos.length === 0 ? (
          <p className="text-zinc-500 text-sm">
            No repositories added yet. Search GitHub repos below.
          </p>
        ) : (
          <div className="space-y-2">
            {localRepos.map((repo) => (
              <div
                key={repo.id}
                className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between p-4 rounded-lg border border-zinc-800 bg-zinc-900"
              >
                <div>
                  <p className="font-medium break-all">
                    {repo.repo_type === "local"
                      ? repo.name
                      : `${repo.owner}/${repo.name}`}
                    {repo.repo_type === "local" && (
                      <span className="ml-2 text-xs text-zinc-500 border border-zinc-700 px-1.5 py-0.5 rounded">
                        local
                      </span>
                    )}
                  </p>
                  {repo.repo_type === "local" && repo.source_path && (
                    <p className="text-xs text-zinc-600 break-all">
                      {repo.source_path}
                    </p>
                  )}
                  <p className="text-xs text-zinc-500">
                    {repo.clone_status === "ready"
                      ? `Ready — ${repo.default_branch}`
                      : repo.clone_status === "cloning"
                        ? "Cloning..."
                        : `Error`}
                    {repo.last_synced &&
                      ` — synced ${new Date(repo.last_synced).toLocaleString()}`}
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <StatusDot status={repo.clone_status} />
                  {repo.clone_status === "ready" && (
                    <button
                      onClick={() => syncRepo(repo.id)}
                      className="text-xs text-zinc-400 hover:text-white px-3 py-1.5 rounded border border-zinc-700 hover:border-zinc-600 transition-colors"
                    >
                      Sync
                    </button>
                  )}
                  <button
                    onClick={() => removeRepo(repo.id)}
                    className="text-xs text-zinc-400 hover:text-red-400 px-3 py-1.5 rounded border border-zinc-700 hover:border-red-800 transition-colors"
                  >
                    Remove
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Add local folder */}
      <div className="mb-6 sm:mb-8">
        <h3 className="text-sm font-medium text-zinc-400 uppercase tracking-wider mb-3">
          Add Local Folder
        </h3>
        {localError && (
          <div className="mb-3 p-3 bg-red-900/30 border border-red-800 rounded-md text-sm text-red-300">
            {localError}
          </div>
        )}
        <div className="flex flex-col gap-2 sm:flex-row">
          <input
            type="text"
            value={localPath}
            onChange={(e) => setLocalPath(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && addLocalFolder()}
            placeholder="/path/to/git/repo"
            className="flex-1 px-3 py-2.5 bg-zinc-900 border border-zinc-700 rounded-md text-sm placeholder-zinc-600 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
          <button
            onClick={addLocalFolder}
            disabled={!localPath.trim()}
            className="text-xs bg-blue-600 hover:bg-blue-500 text-white px-4 py-2.5 rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            Add
          </button>
        </div>
      </div>

      {/* GitHub repos */}
      <div>
        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between mb-3">
          <h3 className="text-sm font-medium text-zinc-400 uppercase tracking-wider">
            GitHub Repositories
          </h3>
          <button
            onClick={() => searchGitHub(search, true)}
            disabled={loading}
            className="self-start sm:self-auto text-xs text-zinc-400 hover:text-white px-3 py-1.5 rounded border border-zinc-700 hover:border-zinc-600 transition-colors disabled:opacity-50"
          >
            Refresh
          </button>
        </div>

        <div className="relative mb-3">
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search your GitHub repos..."
            className="w-full px-3 py-2.5 bg-zinc-900 border border-zinc-700 rounded-md text-sm placeholder-zinc-600 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
          {loading && (
            <div className="absolute right-3 top-1/2 -translate-y-1/2">
              <div className="w-4 h-4 border-2 border-zinc-600 border-t-zinc-300 rounded-full animate-spin" />
            </div>
          )}
        </div>

        <div className="space-y-2 max-h-[52vh] sm:max-h-96 overflow-auto">
          {filteredGh.map((repo) => (
            <div
              key={repo.full_name}
              className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between p-3 rounded-lg border border-zinc-800 bg-zinc-900/50"
            >
              <div>
                <p className="text-sm font-medium break-all">
                  {repo.full_name}
                </p>
                {repo.description && (
                  <p className="text-xs text-zinc-500 sm:truncate sm:max-w-md">
                    {repo.description}
                  </p>
                )}
              </div>
              <div className="flex flex-wrap items-center gap-2">
                {repo.private && (
                  <span className="text-xs text-zinc-500 border border-zinc-700 px-2 py-0.5 rounded">
                    private
                  </span>
                )}
                <button
                  onClick={() => addRepo(repo.html_url)}
                  className="text-xs bg-blue-600 hover:bg-blue-500 text-white px-3 py-1.5 rounded transition-colors"
                >
                  Add
                </button>
              </div>
            </div>
          ))}
          {!loading && hasLoaded && filteredGh.length === 0 && (
            <p className="text-zinc-500 text-sm py-4 text-center">
              {search
                ? "No repos found. Try a different search."
                : "No repos to show."}
            </p>
          )}
        </div>
      </div>
    </div>
  );
}

function StatusDot({ status }: { status: string }) {
  const color =
    status === "ready"
      ? "bg-emerald-500"
      : status === "cloning"
        ? "bg-amber-500 animate-pulse"
        : "bg-red-500";
  return <div className={`w-2.5 h-2.5 rounded-full ${color}`} />;
}
