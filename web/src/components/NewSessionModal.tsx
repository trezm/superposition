import { useEffect, useState } from "react";
import { api } from "../lib/api";

interface Repo {
  id: number;
  owner: string;
  name: string;
  clone_status: string;
  default_branch: string;
}

interface Props {
  open: boolean;
  onClose: () => void;
  onCreated: (session: any) => void;
}

export default function NewSessionModal({ open, onClose, onCreated }: Props) {
  const [repos, setRepos] = useState<Repo[]>([]);
  const [repoId, setRepoId] = useState<number | null>(null);
  const [sourceBranch, setSourceBranch] = useState("");
  const [newBranch, setNewBranch] = useState("");
  const [branches, setBranches] = useState<string[]>([]);
  const [cliType, setCliType] = useState<"claude" | "codex" | "gemini">("claude");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (open) {
      api.getRepos().then((repos) => {
        const ready = repos.filter((r: Repo) => r.clone_status === "ready");
        setRepos(ready);
        if (ready.length > 0 && !repoId) {
          setRepoId(ready[0].id);
        }
      });
    }
  }, [open]);

  useEffect(() => {
    if (repoId) {
      api.getRepoBranches(repoId).then((b) => {
        setBranches(b);
        const repo = repos.find((r) => r.id === repoId);
        const defaultBranch = repo?.default_branch || b[0] || "main";
        setSourceBranch(defaultBranch);
        setNewBranch("");
      });
    }
  }, [repoId]);

  if (!open) return null;

  const handleSubmit = async () => {
    if (!repoId || !sourceBranch || !newBranch.trim()) return;
    setLoading(true);
    setError("");
    try {
      const session = await api.createSession(
        repoId,
        sourceBranch,
        newBranch.trim(),
        cliType,
      );
      onCreated(session);
      onClose();
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/60 flex items-center justify-center z-50">
      <div className="bg-zinc-900 border border-zinc-800 rounded-xl p-6 w-full max-w-md">
        <h3 className="text-lg font-bold mb-4">New Session</h3>

        {error && (
          <div className="mb-4 p-2 bg-red-900/30 border border-red-800 rounded text-sm text-red-300">
            {error}
          </div>
        )}

        <div className="space-y-4">
          <div>
            <label className="block text-sm font-medium mb-1">Repository</label>
            {repos.length === 0 ? (
              <p className="text-sm text-zinc-500">
                No repos available. Add one first.
              </p>
            ) : (
              <select
                value={repoId ?? ""}
                onChange={(e) => setRepoId(Number(e.target.value))}
                className="w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-md text-sm"
              >
                {repos.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.owner}/{r.name}
                  </option>
                ))}
              </select>
            )}
          </div>

          <div>
            <label className="block text-sm font-medium mb-1">
              Source Branch
            </label>
            <select
              value={sourceBranch}
              onChange={(e) => setSourceBranch(e.target.value)}
              className="w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-md text-sm"
            >
              {branches.map((b) => (
                <option key={b} value={b}>
                  {b}
                </option>
              ))}
            </select>
            <p className="text-xs text-zinc-500 mt-1">
              The branch to base your new branch on.
            </p>
          </div>

          <div>
            <label className="block text-sm font-medium mb-1">
              New Branch Name
            </label>
            <input
              type="text"
              value={newBranch}
              onChange={(e) => setNewBranch(e.target.value)}
              placeholder="e.g. feat/add-login-page"
              className="w-full px-3 py-2 bg-zinc-800 border border-zinc-700 rounded-md text-sm placeholder-zinc-600 focus:outline-none focus:ring-1 focus:ring-blue-500"
            />
          </div>

          <div>
            <label className="block text-sm font-medium mb-1">CLI</label>
            <div className="flex gap-2">
              {(["claude", "codex", "gemini"] as const).map((type) => (
                <button
                  key={type}
                  onClick={() => setCliType(type)}
                  className={`flex-1 px-3 py-2 rounded-md text-sm font-medium transition-colors ${
                    cliType === type
                      ? "bg-blue-600 text-white"
                      : "bg-zinc-800 text-zinc-400 hover:text-white"
                  }`}
                >
                  {type === "claude" ? "Claude Code" : type === "codex" ? "Codex" : "Gemini CLI"}
                </button>
              ))}
            </div>
          </div>
        </div>

        <div className="flex justify-end gap-2 mt-6">
          <button
            onClick={onClose}
            className="px-4 py-2 text-sm text-zinc-400 hover:text-white transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={handleSubmit}
            disabled={loading || !repoId || !sourceBranch || !newBranch.trim()}
            className="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-md transition-colors disabled:opacity-50"
          >
            {loading ? "Creating..." : "Create Session"}
          </button>
        </div>
      </div>
    </div>
  );
}
