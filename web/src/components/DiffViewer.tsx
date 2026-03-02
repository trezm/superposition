import { useEffect, useState, useCallback, useMemo } from "react";
import {
  api,
  type DiffResponse,
  type DiffFile,
  type DiffHunk,
  type DiffLine,
} from "../lib/api";
import { extToLang, tokenizeLines } from "../lib/highlighter";
import { useToast } from "./Toast";

type ViewMode = "unified" | "split";

interface TokenSpan {
  content: string;
  color?: string;
}

interface ReviewComment {
  filePath: string;
  lineNum: number;
  lineContent: string;
  body: string;
}

interface ReviewCallbacks {
  comments: Map<string, ReviewComment>;
  activeForm: string | null;
  onOpenForm: (
    key: string,
    filePath: string,
    lineNum: number,
    lineContent: string,
  ) => void;
  onSaveComment: (key: string, body: string) => void;
  onDeleteComment: (key: string) => void;
  onCancelForm: () => void;
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

  // Review state
  const [comments, setComments] = useState<Map<string, ReviewComment>>(
    new Map(),
  );
  const [activeForm, setActiveForm] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const { toast } = useToast();

  const fetchDiff = useCallback(async () => {
    setLoading(true);
    setError(null);
    setComments(new Map());
    setActiveForm(null);
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

  const handleOpenForm = (
    key: string,
    filePath: string,
    lineNum: number,
    lineContent: string,
  ) => {
    // If there's already a comment for this line, open it for editing
    if (comments.has(key)) {
      setActiveForm(key);
      return;
    }
    // Pre-populate the comment metadata (body will be filled on save)
    setComments((prev) => {
      const next = new Map(prev);
      next.set(key, { filePath, lineNum, lineContent, body: "" });
      return next;
    });
    setActiveForm(key);
  };

  const handleSaveComment = (key: string, body: string) => {
    setComments((prev) => {
      const next = new Map(prev);
      const existing = next.get(key);
      if (existing) {
        next.set(key, { ...existing, body });
      }
      return next;
    });
    setActiveForm(null);
  };

  const handleDeleteComment = (key: string) => {
    setComments((prev) => {
      const next = new Map(prev);
      next.delete(key);
      return next;
    });
    if (activeForm === key) setActiveForm(null);
  };

  const handleCancelForm = () => {
    // If the comment has no body yet, remove it entirely
    if (activeForm) {
      const comment = comments.get(activeForm);
      if (comment && !comment.body) {
        setComments((prev) => {
          const next = new Map(prev);
          next.delete(activeForm);
          return next;
        });
      }
    }
    setActiveForm(null);
  };

  const handleSubmitReview = async () => {
    // Filter out comments with empty bodies
    const validComments = Array.from(comments.values()).filter((c) => c.body);
    if (validComments.length === 0) return;

    // Group by file path
    const grouped = new Map<string, ReviewComment[]>();
    for (const c of validComments) {
      const existing = grouped.get(c.filePath) || [];
      existing.push(c);
      grouped.set(c.filePath, existing);
    }

    // Sort each group by line number
    for (const [, group] of grouped) {
      group.sort((a, b) => a.lineNum - b.lineNum);
    }

    // Build the review message
    let message = "I've reviewed the diff. Here are my comments:\n";
    for (const [filePath, group] of grouped) {
      message += `\n## ${filePath}\n`;
      for (const c of group) {
        const trimmed = c.lineContent.trim();
        message += `\n**Line ${c.lineNum}**${trimmed ? ` (\`${trimmed}\`)` : ""}:\n> ${c.body}\n`;
      }
    }
    message += "\nPlease address these review comments.\n";

    setSubmitting(true);
    try {
      await api.sendSessionInput(sessionId, message);
      setComments(new Map());
      setActiveForm(null);
      toast("Review submitted", "success");
    } catch (e: any) {
      toast(e.message || "Failed to submit review", "error");
    } finally {
      setSubmitting(false);
    }
  };

  const reviewCallbacks: ReviewCallbacks = {
    comments,
    activeForm,
    onOpenForm: handleOpenForm,
    onSaveComment: handleSaveComment,
    onDeleteComment: handleDeleteComment,
    onCancelForm: handleCancelForm,
  };

  // Count comments with actual bodies
  const commentCount = Array.from(comments.values()).filter(
    (c) => c.body,
  ).length;

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

        {commentCount > 0 && (
          <div className="flex items-center gap-2">
            <span className="text-xs text-zinc-400">
              {commentCount} comment{commentCount !== 1 ? "s" : ""}
            </span>
            <button
              onClick={handleSubmitReview}
              disabled={submitting}
              className="text-xs px-3 py-1 rounded bg-blue-600 hover:bg-blue-500 text-white font-medium transition-colors disabled:opacity-50"
            >
              {submitting ? "Submitting..." : "Submit Review"}
            </button>
          </div>
        )}

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
            review={reviewCallbacks}
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
  review,
}: {
  file: DiffFile;
  viewMode: ViewMode;
  isCollapsed: boolean;
  onToggle: () => void;
  review: ReviewCallbacks;
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
            <UnifiedView file={file} review={review} />
          ) : (
            <SplitView file={file} review={review} />
          )}
        </div>
      )}
    </div>
  );
}

// Syntax highlighting hook for a whole file's lines
function useHighlightedLines(file: DiffFile) {
  const [tokenMap, setTokenMap] = useState<Map<string, TokenSpan[]>>(new Map());

  const allLines = useMemo(() => {
    const lines: string[] = [];
    for (const hunk of file.hunks) {
      for (const line of hunk.lines) {
        lines.push(line.content);
      }
    }
    return lines;
  }, [file.hunks]);

  useEffect(() => {
    const lang = extToLang(file.path);
    if (!lang || allLines.length === 0) return;

    let cancelled = false;
    const code = allLines.join("\n");

    tokenizeLines(code, lang).then((result) => {
      if (cancelled) return;
      const map = new Map<string, TokenSpan[]>();
      for (let i = 0; i < result.length && i < allLines.length; i++) {
        // Use index+content as key to handle duplicate lines
        map.set(`${i}:${allLines[i]}`, result[i]);
      }
      setTokenMap(map);
    });

    return () => {
      cancelled = true;
    };
  }, [file.path, allLines]);

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

/** Returns the comment key for a diff line: filePath:lineNum */
function commentKey(filePath: string, line: DiffLine): string {
  // Use new_num for add/context lines, old_num for delete-only lines
  const num = line.type === "delete" ? line.old_num : line.new_num;
  return `${filePath}:${num}`;
}

function InlineCommentForm({
  commentKey: key,
  existingBody,
  onSave,
  onCancel,
  colSpan,
}: {
  commentKey: string;
  existingBody: string;
  onSave: (key: string, body: string) => void;
  onCancel: () => void;
  colSpan: number;
}) {
  const [body, setBody] = useState(existingBody);

  return (
    <tr key={`form-${key}`}>
      <td colSpan={colSpan} className="px-4 py-2 bg-zinc-800/50">
        <div className="border border-zinc-600 rounded overflow-hidden">
          <textarea
            autoFocus
            rows={3}
            value={body}
            onChange={(e) => setBody(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                e.preventDefault();
                if (body.trim()) onSave(key, body.trim());
              }
              if (e.key === "Escape") {
                e.preventDefault();
                onCancel();
              }
            }}
            className="w-full bg-zinc-900 text-zinc-200 text-xs p-2 resize-none outline-none placeholder-zinc-600"
            placeholder="Write a review comment..."
          />
          <div className="flex items-center gap-2 px-2 py-1.5 bg-zinc-850 border-t border-zinc-700">
            <button
              onClick={() => {
                if (body.trim()) onSave(key, body.trim());
              }}
              disabled={!body.trim()}
              className="text-xs px-3 py-1 rounded bg-blue-600 hover:bg-blue-500 text-white font-medium transition-colors disabled:opacity-50"
            >
              Comment
            </button>
            <button
              onClick={onCancel}
              className="text-xs px-3 py-1 rounded text-zinc-400 hover:text-white transition-colors"
            >
              Cancel
            </button>
            <span className="text-[10px] text-zinc-600 ml-auto">
              Ctrl+Enter to save
            </span>
          </div>
        </div>
      </td>
    </tr>
  );
}

function SavedCommentRow({
  commentKey: key,
  comment,
  onEdit,
  onDelete,
  colSpan,
}: {
  commentKey: string;
  comment: ReviewComment;
  onEdit: (
    key: string,
    filePath: string,
    lineNum: number,
    lineContent: string,
  ) => void;
  onDelete: (key: string) => void;
  colSpan: number;
}) {
  return (
    <tr key={`comment-${key}`}>
      <td colSpan={colSpan} className="px-4 py-2">
        <div className="border border-blue-800/50 rounded bg-blue-950/30 p-2">
          <p className="text-xs text-zinc-200 whitespace-pre-wrap">
            {comment.body}
          </p>
          <div className="flex items-center gap-2 mt-1.5">
            <button
              onClick={() =>
                onEdit(
                  key,
                  comment.filePath,
                  comment.lineNum,
                  comment.lineContent,
                )
              }
              className="text-[10px] text-zinc-500 hover:text-blue-400 transition-colors"
            >
              Edit
            </button>
            <button
              onClick={() => onDelete(key)}
              className="text-[10px] text-zinc-500 hover:text-red-400 transition-colors"
            >
              Delete
            </button>
          </div>
        </div>
      </td>
    </tr>
  );
}

function AddCommentButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      onClick={(e) => {
        e.stopPropagation();
        onClick();
      }}
      className="absolute left-0 top-0 w-5 h-full flex items-center justify-center opacity-0 group-hover/line:opacity-100 bg-blue-600 hover:bg-blue-500 text-white text-xs font-bold transition-opacity cursor-pointer z-10"
      title="Add review comment"
    >
      +
    </button>
  );
}

function UnifiedView({
  file,
  review,
}: {
  file: DiffFile;
  review: ReviewCallbacks;
}) {
  const tokenMap = useHighlightedLines(file);

  let lineIdx = 0;

  return (
    <table className="w-full text-xs font-mono border-collapse">
      <tbody>
        {file.hunks.map((hunk, hi) => (
          <HunkRows
            key={hi}
            hunk={hunk}
            filePath={file.path}
            tokenMap={tokenMap}
            lineIdxRef={{ current: lineIdx }}
            onAdvance={(n) => {
              lineIdx += n;
            }}
            unified
            review={review}
          />
        ))}
      </tbody>
    </table>
  );
}

function HunkRows({
  hunk,
  filePath,
  tokenMap,
  lineIdxRef,
  onAdvance,
  unified,
  review,
}: {
  hunk: DiffHunk;
  filePath: string;
  tokenMap: Map<string, TokenSpan[]>;
  lineIdxRef: { current: number };
  onAdvance: (n: number) => void;
  unified?: boolean;
  review: ReviewCallbacks;
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
    const idx = lineIdxRef.current;
    lineIdxRef.current++;

    const tokens = tokenMap.get(`${idx}:${line.content}`);
    const bgClass =
      line.type === "add"
        ? "bg-emerald-500/10"
        : line.type === "delete"
          ? "bg-red-500/10"
          : "";

    if (unified) {
      const key = commentKey(filePath, line);
      const lineNum = line.type === "delete" ? line.old_num : line.new_num;
      const hasComment =
        review.comments.has(key) && review.comments.get(key)!.body;
      const isFormOpen = review.activeForm === key;

      rows.push(
        <tr key={`${idx}-${i}`} className={`${bgClass} group/line relative`}>
          <td className="w-10 text-right pr-2 text-zinc-600 select-none align-top relative">
            {lineNum && (
              <AddCommentButton
                onClick={() =>
                  review.onOpenForm(key, filePath, lineNum, line.content)
                }
              />
            )}
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

      // Comment form or saved comment
      if (isFormOpen) {
        rows.push(
          <InlineCommentForm
            key={`form-${key}`}
            commentKey={key}
            existingBody={review.comments.get(key)?.body || ""}
            onSave={review.onSaveComment}
            onCancel={review.onCancelForm}
            colSpan={3}
          />,
        );
      } else if (hasComment) {
        rows.push(
          <SavedCommentRow
            key={`comment-${key}`}
            commentKey={key}
            comment={review.comments.get(key)!}
            onEdit={review.onOpenForm}
            onDelete={review.onDeleteComment}
            colSpan={3}
          />,
        );
      }
    }
  });

  onAdvance(0); // noop, counter already advanced
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

function SplitView({
  file,
  review,
}: {
  file: DiffFile;
  review: ReviewCallbacks;
}) {
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

            const key = line?.old_num ? `${file.path}:${line.old_num}` : null;
            const hasComment =
              key && review.comments.has(key) && review.comments.get(key)!.body;
            const isFormOpen = key && review.activeForm === key;

            return (
              <SplitLineRows
                key={i}
                line={line}
                lineNum={line?.old_num}
                bgClass={bgClass}
                tokens={tokens}
                commentKey={key}
                hasComment={!!hasComment}
                isFormOpen={!!isFormOpen}
                filePath={file.path}
                review={review}
              />
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

            const key = line?.new_num ? `${file.path}:${line.new_num}` : null;
            const hasComment =
              key && review.comments.has(key) && review.comments.get(key)!.body;
            const isFormOpen = key && review.activeForm === key;

            return (
              <SplitLineRows
                key={i}
                line={line}
                lineNum={line?.new_num}
                bgClass={bgClass}
                tokens={tokens}
                commentKey={key}
                hasComment={!!hasComment}
                isFormOpen={!!isFormOpen}
                filePath={file.path}
                review={review}
              />
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function SplitLineRows({
  line,
  lineNum,
  bgClass,
  tokens,
  commentKey: key,
  hasComment,
  isFormOpen,
  filePath,
  review,
}: {
  line: DiffLine | null;
  lineNum: number | undefined;
  bgClass: string;
  tokens: TokenSpan[] | undefined;
  commentKey: string | null;
  hasComment: boolean;
  isFormOpen: boolean;
  filePath: string;
  review: ReviewCallbacks;
}) {
  return (
    <>
      <tr className={`${bgClass} group/line relative`}>
        <td className="w-10 text-right pr-2 text-zinc-600 select-none align-top relative">
          {key && lineNum && (
            <AddCommentButton
              onClick={() =>
                review.onOpenForm(key, filePath, lineNum, line?.content || "")
              }
            />
          )}
          {lineNum || ""}
        </td>
        <td className="pl-2 py-0 whitespace-pre">
          {line ? renderTokens(tokens, line.content) : ""}
        </td>
      </tr>
      {isFormOpen && key && (
        <InlineCommentForm
          commentKey={key}
          existingBody={review.comments.get(key)?.body || ""}
          onSave={review.onSaveComment}
          onCancel={review.onCancelForm}
          colSpan={2}
        />
      )}
      {!isFormOpen && hasComment && key && (
        <SavedCommentRow
          commentKey={key}
          comment={review.comments.get(key)!}
          onEdit={review.onOpenForm}
          onDelete={review.onDeleteComment}
          colSpan={2}
        />
      )}
    </>
  );
}
