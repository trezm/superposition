import { useEffect, useState } from "react";
import { api } from "../lib/api";
import { useToast } from "../components/Toast";

export default function Settings() {
  const [pat, setPat] = useState("");
  const [loading, setLoading] = useState(true);
  const { toast } = useToast();

  useEffect(() => {
    api
      .getSetting("github_pat")
      .then((s) => setPat(s.value))
      .catch(() => {})
      .finally(() => setLoading(false));
  }, []);

  const handleSave = async () => {
    try {
      if (pat.trim()) {
        await api.putSetting("github_pat", pat.trim());
      } else {
        await api.deleteSetting("github_pat").catch(() => {});
      }
      toast("Settings saved", "success");
    } catch (e: any) {
      toast(e.message, "error");
    }
  };

  return (
    <div className="w-full max-w-2xl p-4 sm:p-6 lg:p-8">
      <h2 className="text-xl sm:text-2xl font-bold mb-1">Settings</h2>
      <p className="text-sm sm:text-base text-zinc-400 mb-6 sm:mb-8">
        Configure your GitHub access and preferences.
      </p>

      <div className="space-y-6">
        <div>
          <label className="block text-sm font-medium mb-2">
            GitHub Personal Access Token
          </label>
          <p className="text-xs text-zinc-500 mb-3">
            Required for accessing repositories. Use a{" "}
            <a
              href="https://github.com/settings/tokens"
              target="_blank"
              rel="noopener noreferrer"
              className="text-blue-400 hover:underline"
            >
              classic token
            </a>{" "}
            with <code className="text-zinc-400">repo</code> scope. Fine-grained
            tokens only cover a single account or org — a classic token is
            needed to access repos across all your orgs.
          </p>
          <input
            type="password"
            value={pat}
            onChange={(e) => setPat(e.target.value)}
            placeholder={loading ? "Loading..." : "ghp_xxxxxxxxxxxxxxxxxxxx"}
            className="w-full px-3 py-2.5 bg-zinc-900 border border-zinc-700 rounded-md text-sm placeholder-zinc-600 focus:outline-none focus:ring-1 focus:ring-blue-500 focus:border-blue-500"
          />
        </div>
        <button
          onClick={handleSave}
          className="w-full sm:w-auto px-4 py-2.5 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-md transition-colors"
        >
          Save
        </button>
      </div>
    </div>
  );
}
