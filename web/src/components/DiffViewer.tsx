import { useState, useEffect, useCallback, useRef } from "react";
import { api } from "../lib/api";
import type { DiffFile, DiffResponse } from "../lib/api";

interface DiffViewerProps {
  sessionId: string;
  visible?: boolean;
}

const STATUS_LABELS: Record<string, string> = {
  added: "A",
  modified: "M",
  deleted: "D",
  renamed: "R",
};

const STATUS_COLORS: Record<string, string> = {
  added: "bg-green-900/60 text-green-400",
  modified: "bg-blue-900/60 text-blue-400",
  deleted: "bg-red-900/60 text-red-400",
  renamed: "bg-yellow-900/60 text-yellow-400",
};

export default function DiffViewer({
  sessionId,
  visible = true,
}: DiffViewerProps) {
  const [data, setData] = useState<DiffResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});
  const hasFetched = useRef(false);

  const fetchDiff = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const resp = await api.getSessionDiff(sessionId);
      setData(resp);
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : "Failed to load diff");
    } finally {
      setLoading(false);
    }
  }, [sessionId]);

  useEffect(() => {
    if (visible && !hasFetched.current) {
      hasFetched.current = true;
      fetchDiff();
    }
  }, [visible, fetchDiff]);

  const toggleFile = (path: string) => {
    setCollapsed((prev) => ({ ...prev, [path]: !prev[path] }));
  };

  if (!visible) return null;

  if (loading && !data) {
    return (
      <div className="flex items-center justify-center h-full text-zinc-500 text-sm">
        Loading diff...
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-3">
        <p className="text-zinc-500 text-sm">{error}</p>
        <button
          onClick={fetchDiff}
          className="text-xs text-blue-400 hover:text-blue-300 px-3 py-1.5 rounded border border-zinc-700 hover:border-blue-800 transition-colors"
        >
          Retry
        </button>
      </div>
    );
  }

  if (!data || data.files.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-3">
        <p className="text-zinc-500 text-sm">No changes yet</p>
        <button
          onClick={fetchDiff}
          className="text-xs text-blue-400 hover:text-blue-300 px-3 py-1.5 rounded border border-zinc-700 hover:border-blue-800 transition-colors"
        >
          Refresh
        </button>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col min-h-0">
      {/* Stats bar */}
      <div className="flex items-center gap-3 px-4 py-2.5 border-b border-zinc-800 bg-zinc-900/80 shrink-0">
        <span className="text-sm text-zinc-400">
          {data.stats.files_changed} file
          {data.stats.files_changed !== 1 ? "s" : ""} changed
        </span>
        {data.stats.insertions > 0 && (
          <span className="text-sm text-green-400">
            +{data.stats.insertions}
          </span>
        )}
        {data.stats.deletions > 0 && (
          <span className="text-sm text-red-400">-{data.stats.deletions}</span>
        )}
        <div className="flex-1" />
        <button
          onClick={fetchDiff}
          disabled={loading}
          className="text-xs text-zinc-400 hover:text-white px-3 py-1.5 rounded border border-zinc-700 hover:border-zinc-600 transition-colors disabled:opacity-50"
        >
          {loading ? "Loading..." : "Refresh"}
        </button>
      </div>

      {/* File list */}
      <div className="flex-1 overflow-y-auto">
        {data.files.map((file) => (
          <FileSection
            key={file.path}
            file={file}
            collapsed={!!collapsed[file.path]}
            onToggle={() => toggleFile(file.path)}
          />
        ))}
      </div>
    </div>
  );
}

function FileSection({
  file,
  collapsed,
  onToggle,
}: {
  file: DiffFile;
  collapsed: boolean;
  onToggle: () => void;
}) {
  return (
    <div className="border-b border-zinc-800">
      {/* File header */}
      <button
        onClick={onToggle}
        className="w-full flex items-center gap-2 px-4 py-2 bg-zinc-900/50 hover:bg-zinc-800/50 transition-colors text-left"
      >
        <span className="text-zinc-500 text-xs shrink-0">
          {collapsed ? "▶" : "▼"}
        </span>
        <span
          className={`text-[10px] font-bold px-1.5 py-0.5 rounded shrink-0 ${STATUS_COLORS[file.status] || "bg-zinc-800 text-zinc-400"}`}
        >
          {STATUS_LABELS[file.status] || "?"}
        </span>
        <span className="text-sm text-zinc-200 font-mono truncate">
          {file.old_path && file.old_path !== file.path
            ? `${file.old_path} → ${file.path}`
            : file.path}
        </span>
        <div className="flex-1" />
        {file.additions > 0 && (
          <span className="text-xs text-green-400 shrink-0">
            +{file.additions}
          </span>
        )}
        {file.deletions > 0 && (
          <span className="text-xs text-red-400 shrink-0 ml-1">
            -{file.deletions}
          </span>
        )}
      </button>

      {/* Hunks */}
      {!collapsed && (
        <div className="font-mono text-xs leading-5">
          {file.hunks.map((hunk, hi) => (
            <div key={hi}>
              {/* Hunk header */}
              <div className="px-4 py-1 bg-blue-950/30 text-blue-400 select-none">
                {hunk.header}
              </div>
              {/* Lines */}
              {hunk.lines.map((line, li) => {
                const bgClass =
                  line.type === "add"
                    ? "bg-green-950/40"
                    : line.type === "delete"
                      ? "bg-red-950/40"
                      : "";
                const textClass =
                  line.type === "add"
                    ? "text-green-400"
                    : line.type === "delete"
                      ? "text-red-400"
                      : "text-zinc-400";
                const prefix =
                  line.type === "add"
                    ? "+"
                    : line.type === "delete"
                      ? "-"
                      : " ";
                return (
                  <div
                    key={li}
                    className={`flex ${bgClass} hover:brightness-125`}
                  >
                    <span className="w-12 shrink-0 text-right pr-2 text-zinc-600 select-none">
                      {line.old_no || ""}
                    </span>
                    <span className="w-12 shrink-0 text-right pr-2 text-zinc-600 select-none border-r border-zinc-800">
                      {line.new_no || ""}
                    </span>
                    <span className={`pl-2 whitespace-pre ${textClass}`}>
                      {prefix}
                      {line.content}
                    </span>
                  </div>
                );
              })}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
