import { useEffect, useState, useCallback, useMemo } from "react";
import {
  api,
  type DiffResponse,
  type DiffFile,
  type DiffHunk,
  type DiffLine,
} from "../lib/api";
import { extToLang, tokenizeLines } from "../lib/highlighter";

type ViewMode = "unified" | "split";

interface TokenSpan {
  content: string;
  color?: string;
}

export default function DiffViewer({
  sessionId,
  visible,
}: {
  sessionId: string;
  visible: boolean;
}) {
  const [diff, setDiff] = useState<DiffResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<ViewMode>("unified");
  const [collapsed, setCollapsed] = useState<Record<string, boolean>>({});

  const fetchDiff = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const data = await api.getSessionDiff(sessionId);
      setDiff(data);
    } catch (e: any) {
      setError(e.message || "Failed to load diff");
    } finally {
      setLoading(false);
    }
  }, [sessionId]);

  useEffect(() => {
    if (visible) fetchDiff();
  }, [visible, fetchDiff]);

  const toggleCollapse = (path: string) => {
    setCollapsed((prev) => ({ ...prev, [path]: !prev[path] }));
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center h-full text-zinc-400">
        <svg
          className="animate-spin h-5 w-5 mr-2"
          viewBox="0 0 24 24"
          fill="none"
        >
          <circle
            className="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            strokeWidth="4"
          />
          <path
            className="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
          />
        </svg>
        Loading diff...
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-zinc-400 gap-3">
        <p className="text-red-400">{error}</p>
        <button
          onClick={fetchDiff}
          className="text-xs px-3 py-1.5 rounded border border-zinc-700 hover:border-zinc-500 transition-colors"
        >
          Retry
        </button>
      </div>
    );
  }

  if (!diff || !diff.files?.length) {
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
      {/* Header bar */}
      <div className="shrink-0 flex items-center gap-3 px-4 py-2 border-b border-zinc-800 bg-zinc-900/80 text-sm">
        <span className="text-zinc-400">
          {diff.stats.files_changed} file
          {diff.stats.files_changed !== 1 ? "s" : ""} changed
        </span>
        <span className="text-emerald-400">+{diff.stats.additions}</span>
        <span className="text-red-400">-{diff.stats.deletions}</span>

        <div className="flex-1" />

        <div className="flex items-center rounded border border-zinc-700 overflow-hidden">
          <button
            onClick={() => setViewMode("unified")}
            className={`px-2.5 py-1 text-xs transition-colors ${
              viewMode === "unified"
                ? "bg-zinc-700 text-white"
                : "text-zinc-400 hover:text-white"
            }`}
          >
            Unified
          </button>
          <button
            onClick={() => setViewMode("split")}
            className={`px-2.5 py-1 text-xs transition-colors ${
              viewMode === "split"
                ? "bg-zinc-700 text-white"
                : "text-zinc-400 hover:text-white"
            }`}
          >
            Split
          </button>
        </div>

        <button
          onClick={fetchDiff}
          className="text-xs text-zinc-400 hover:text-white px-2 py-1 rounded border border-zinc-700 hover:border-zinc-500 transition-colors"
        >
          Refresh
        </button>
      </div>

      {/* File list */}
      <div className="flex-1 overflow-y-auto min-h-0">
        {diff.files.map((file) => (
          <FileSection
            key={file.path}
            file={file}
            viewMode={viewMode}
            isCollapsed={collapsed[file.path] ?? false}
            onToggle={() => toggleCollapse(file.path)}
          />
        ))}
      </div>
    </div>
  );
}

function statusBadge(status: string) {
  const map: Record<string, { label: string; color: string }> = {
    added: { label: "A", color: "text-emerald-400 border-emerald-800" },
    modified: { label: "M", color: "text-blue-400 border-blue-800" },
    deleted: { label: "D", color: "text-red-400 border-red-800" },
    renamed: { label: "R", color: "text-amber-400 border-amber-800" },
  };
  const badge = map[status] ?? {
    label: status[0]?.toUpperCase() ?? "?",
    color: "text-zinc-400 border-zinc-700",
  };
  return (
    <span
      className={`inline-flex items-center justify-center w-5 h-5 text-[10px] font-bold rounded border ${badge.color}`}
    >
      {badge.label}
    </span>
  );
}

function FileSection({
  file,
  viewMode,
  isCollapsed,
  onToggle,
}: {
  file: DiffFile;
  viewMode: ViewMode;
  isCollapsed: boolean;
  onToggle: () => void;
}) {
  return (
    <div className="border-b border-zinc-800">
      {/* File header */}
      <button
        onClick={onToggle}
        className="w-full flex items-center gap-2 px-4 py-2 text-sm hover:bg-zinc-800/50 transition-colors"
      >
        <span className="text-zinc-500 text-xs">{isCollapsed ? "+" : "-"}</span>
        {statusBadge(file.status)}
        <span className="text-zinc-200 font-mono text-xs truncate">
          {file.old_path && file.old_path !== file.path
            ? `${file.old_path} → ${file.path}`
            : file.path}
        </span>
        <span className="ml-auto flex items-center gap-2 shrink-0 text-xs">
          {file.additions > 0 && (
            <span className="text-emerald-400">+{file.additions}</span>
          )}
          {file.deletions > 0 && (
            <span className="text-red-400">-{file.deletions}</span>
          )}
        </span>
      </button>

      {/* File content */}
      {!isCollapsed && (
        <div className="overflow-x-auto">
          {file.binary ? (
            <div className="px-4 py-3 text-xs text-zinc-500">
              Binary file changed
            </div>
          ) : viewMode === "unified" ? (
            <UnifiedView file={file} />
          ) : (
            <SplitView file={file} />
          )}
        </div>
      )}
    </div>
  );
}

// Syntax highlighting hook — tokenizes old (context+delete) and new (context+add)
// streams separately so the highlighter sees valid code for each side.
function useHighlightedLines(file: DiffFile) {
  const [tokenMap, setTokenMap] = useState<Map<string, TokenSpan[]>>(new Map());

  const streams = useMemo(() => {
    const oldLines: string[] = [];
    const newLines: string[] = [];
    const lineInfo: {
      globalIdx: number;
      type: string;
      content: string;
      oldStreamIdx?: number;
      newStreamIdx?: number;
    }[] = [];

    let globalIdx = 0;
    for (const hunk of file.hunks) {
      for (const line of hunk.lines) {
        const info: (typeof lineInfo)[0] = {
          globalIdx,
          type: line.type,
          content: line.content,
        };

        if (line.type === "delete" || line.type === "context") {
          info.oldStreamIdx = oldLines.length;
          oldLines.push(line.content);
        }
        if (line.type === "add" || line.type === "context") {
          info.newStreamIdx = newLines.length;
          newLines.push(line.content);
        }

        lineInfo.push(info);
        globalIdx++;
      }
    }

    return { oldLines, newLines, lineInfo };
  }, [file.hunks]);

  useEffect(() => {
    const lang = extToLang(file.path);
    if (!lang || streams.lineInfo.length === 0) return;

    let cancelled = false;

    Promise.all([
      streams.oldLines.length > 0
        ? tokenizeLines(streams.oldLines.join("\n"), lang)
        : Promise.resolve([]),
      streams.newLines.length > 0
        ? tokenizeLines(streams.newLines.join("\n"), lang)
        : Promise.resolve([]),
    ]).then(([oldTokens, newTokens]) => {
      if (cancelled) return;
      const map = new Map<string, TokenSpan[]>();

      for (const info of streams.lineInfo) {
        if (info.type === "delete" && info.oldStreamIdx != null) {
          const tokens = oldTokens[info.oldStreamIdx];
          if (tokens) map.set(`${info.globalIdx}:${info.content}`, tokens);
        } else if (info.newStreamIdx != null) {
          const tokens = newTokens[info.newStreamIdx];
          if (tokens) map.set(`${info.globalIdx}:${info.content}`, tokens);
        }
      }

      setTokenMap(map);
    });

    return () => {
      cancelled = true;
    };
  }, [file.path, streams]);

  return tokenMap;
}

function renderTokens(tokens: TokenSpan[] | undefined, content: string) {
  if (!tokens) {
    return <span>{content}</span>;
  }
  return (
    <>
      {tokens.map((t, j) => (
        <span key={j} style={t.color ? { color: t.color } : undefined}>
          {t.content}
        </span>
      ))}
    </>
  );
}

function UnifiedView({ file }: { file: DiffFile }) {
  const tokenMap = useHighlightedLines(file);

  const hunkStartIndices = useMemo(() => {
    const indices: number[] = [];
    let offset = 0;
    for (const hunk of file.hunks) {
      indices.push(offset);
      offset += hunk.lines.length;
    }
    return indices;
  }, [file.hunks]);

  return (
    <table className="w-full text-xs font-mono border-collapse">
      <tbody>
        {file.hunks.map((hunk, hi) => (
          <HunkRows
            key={hi}
            hunk={hunk}
            tokenMap={tokenMap}
            startIdx={hunkStartIndices[hi]}
            unified
          />
        ))}
      </tbody>
    </table>
  );
}

function HunkRows({
  hunk,
  tokenMap,
  startIdx,
  unified,
}: {
  hunk: DiffHunk;
  tokenMap: Map<string, TokenSpan[]>;
  startIdx: number;
  unified?: boolean;
}) {
  const rows: React.ReactNode[] = [];

  // Hunk header
  if (unified) {
    rows.push(
      <tr key={`hdr-${hunk.header}`} className="bg-zinc-800/30">
        <td className="w-10 text-right pr-2 text-zinc-600 select-none" />
        <td className="w-10 text-right pr-2 text-zinc-600 select-none" />
        <td className="pl-4 py-0.5 text-zinc-500">{hunk.header}</td>
      </tr>,
    );
  }

  hunk.lines.forEach((line, i) => {
    const idx = startIdx + i;
    const tokens = tokenMap.get(`${idx}:${line.content}`);
    const bgClass =
      line.type === "add"
        ? "bg-emerald-500/10"
        : line.type === "delete"
          ? "bg-red-500/10"
          : "";

    if (unified) {
      rows.push(
        <tr key={`${idx}-${i}`} className={bgClass}>
          <td className="w-10 text-right pr-2 text-zinc-600 select-none align-top">
            {line.old_num || ""}
          </td>
          <td className="w-10 text-right pr-2 text-zinc-600 select-none align-top">
            {line.new_num || ""}
          </td>
          <td className="pl-4 py-0 whitespace-pre">
            <span
              className={
                line.type === "add"
                  ? "text-emerald-300"
                  : line.type === "delete"
                    ? "text-red-300"
                    : ""
              }
            >
              {line.type === "add" ? "+" : line.type === "delete" ? "-" : " "}
            </span>
            {renderTokens(tokens, line.content)}
          </td>
        </tr>,
      );
    }
  });

  return <>{rows}</>;
}

// Side-by-side view
interface SplitRow {
  left: DiffLine | null;
  right: DiffLine | null;
  leftIdx: number;
  rightIdx: number;
}

function buildSplitRows(hunks: DiffHunk[]): {
  rows: SplitRow[];
  lineIndices: { left: number[]; right: number[] };
} {
  const rows: SplitRow[] = [];
  const leftIndices: number[] = [];
  const rightIndices: number[] = [];
  let globalIdx = 0;

  for (const hunk of hunks) {
    const lines = hunk.lines;
    let i = 0;

    while (i < lines.length) {
      const line = lines[i];

      if (line.type === "context") {
        rows.push({
          left: line,
          right: line,
          leftIdx: globalIdx,
          rightIdx: globalIdx,
        });
        leftIndices.push(globalIdx);
        rightIndices.push(globalIdx);
        globalIdx++;
        i++;
      } else if (line.type === "delete") {
        // Collect consecutive deletes and adds
        const deletes: { line: DiffLine; idx: number }[] = [];
        while (i < lines.length && lines[i].type === "delete") {
          deletes.push({ line: lines[i], idx: globalIdx });
          globalIdx++;
          i++;
        }
        const adds: { line: DiffLine; idx: number }[] = [];
        while (i < lines.length && lines[i].type === "add") {
          adds.push({ line: lines[i], idx: globalIdx });
          globalIdx++;
          i++;
        }

        const maxLen = Math.max(deletes.length, adds.length);
        for (let j = 0; j < maxLen; j++) {
          rows.push({
            left: j < deletes.length ? deletes[j].line : null,
            right: j < adds.length ? adds[j].line : null,
            leftIdx: j < deletes.length ? deletes[j].idx : -1,
            rightIdx: j < adds.length ? adds[j].idx : -1,
          });
          leftIndices.push(j < deletes.length ? deletes[j].idx : -1);
          rightIndices.push(j < adds.length ? adds[j].idx : -1);
        }
      } else if (line.type === "add") {
        // Add without preceding delete
        rows.push({
          left: null,
          right: line,
          leftIdx: -1,
          rightIdx: globalIdx,
        });
        leftIndices.push(-1);
        rightIndices.push(globalIdx);
        globalIdx++;
        i++;
      } else {
        globalIdx++;
        i++;
      }
    }
  }

  return { rows, lineIndices: { left: leftIndices, right: rightIndices } };
}

function SplitView({ file }: { file: DiffFile }) {
  const tokenMap = useHighlightedLines(file);
  const { rows } = useMemo(() => buildSplitRows(file.hunks), [file.hunks]);

  return (
    <div className="flex">
      {/* Left panel */}
      <table className="w-1/2 text-xs font-mono border-collapse border-r border-zinc-800">
        <tbody>
          {rows.map((row, i) => {
            const line = row.left;
            const bgClass = line
              ? line.type === "delete"
                ? "bg-red-500/10"
                : line.type === "add"
                  ? "bg-emerald-500/10"
                  : ""
              : "bg-zinc-900/50";

            const tokens =
              line && row.leftIdx >= 0
                ? tokenMap.get(`${row.leftIdx}:${line.content}`)
                : undefined;

            return (
              <tr key={i} className={bgClass}>
                <td className="w-10 text-right pr-2 text-zinc-600 select-none align-top">
                  {line?.old_num || ""}
                </td>
                <td className="pl-2 py-0 whitespace-pre">
                  {line ? renderTokens(tokens, line.content) : ""}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>

      {/* Right panel */}
      <table className="w-1/2 text-xs font-mono border-collapse">
        <tbody>
          {rows.map((row, i) => {
            const line = row.right;
            const bgClass = line
              ? line.type === "add"
                ? "bg-emerald-500/10"
                : line.type === "delete"
                  ? "bg-red-500/10"
                  : ""
              : "bg-zinc-900/50";

            const tokens =
              line && row.rightIdx >= 0
                ? tokenMap.get(`${row.rightIdx}:${line.content}`)
                : undefined;

            return (
              <tr key={i} className={bgClass}>
                <td className="w-10 text-right pr-2 text-zinc-600 select-none align-top">
                  {line?.new_num || ""}
                </td>
                <td className="pl-2 py-0 whitespace-pre">
                  {line ? renderTokens(tokens, line.content) : ""}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
