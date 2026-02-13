import { useEffect, useState, useCallback, useRef } from "react";
import { api } from "../lib/api";
import Terminal from "../components/Terminal";
import NewSessionModal from "../components/NewSessionModal";

interface SessionInfo {
  id: string;
  repo_id: number;
  branch: string;
  cli_type: string;
  status: string;
  created_at: string;
  repo_owner: string;
  repo_name: string;
}

export default function Sessions() {
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [openTabs, setOpenTabs] = useState<string[]>([]);
  const [activeTab, setActiveTab] = useState<string | null>(null);
  const [showModal, setShowModal] = useState(false);
  const [idleSessions, setIdleSessions] = useState<Set<string>>(new Set());
  const notifiedSessions = useRef<Set<string>>(new Set());

  const load = useCallback(() => {
    api.getSessions().then(setSessions).catch(console.error);
  }, []);

  useEffect(() => {
    load();
    const interval = setInterval(load, 5000);
    return () => clearInterval(interval);
  }, [load]);

  // Browser tab title when sessions are idle
  useEffect(() => {
    document.title = idleSessions.size > 0 ? "(!) Superposition" : "Superposition";
  }, [idleSessions]);

  // OS notifications for idle sessions
  useEffect(() => {
    for (const sessionId of idleSessions) {
      if (notifiedSessions.current.has(sessionId)) continue;

      notifiedSessions.current.add(sessionId);
      const session = sessions.find((s) => s.id === sessionId);
      const label = session
        ? `${session.repo_name}/${session.branch}`
        : sessionId;

      if (typeof Notification !== "undefined" && Notification.permission === "granted") {
        const n = new Notification("Superposition", {
          body: `${label} needs your attention`,
        });
        n.onclick = () => {
          window.focus();
          setActiveTab(sessionId);
        };
      } else if (typeof Notification !== "undefined" && Notification.permission === "default") {
        Notification.requestPermission();
      }
    }
  }, [idleSessions, activeTab, sessions]);

  const handleIdleChange = useCallback((sessionId: string, idle: boolean) => {
    setIdleSessions((prev) => {
      const next = new Set(prev);
      if (idle) {
        next.add(sessionId);
      } else {
        next.delete(sessionId);
        notifiedSessions.current.delete(sessionId);
      }
      return next;
    });
  }, []);

  const handleCreated = (session: SessionInfo) => {
    load();
    openTab(session.id);
  };

  const openTab = (id: string) => {
    if (!openTabs.includes(id)) {
      setOpenTabs((prev) => [...prev, id]);
    }
    setActiveTab(id);
  };

  const closeTab = (id: string) => {
    setOpenTabs((prev) => prev.filter((t) => t !== id));
    setIdleSessions((prev) => {
      const next = new Set(prev);
      next.delete(id);
      return next;
    });
    notifiedSessions.current.delete(id);
    if (activeTab === id) {
      const remaining = openTabs.filter((t) => t !== id);
      setActiveTab(
        remaining.length > 0 ? remaining[remaining.length - 1] : null,
      );
    }
  };

  const handleDelete = async (id: string) => {
    const session = sessions.find((s) => s.id === id);
    const target = session
      ? `${session.repo_owner}/${session.repo_name} (${session.branch})`
      : id;
    const deleteLocal = window.confirm(
      `Also delete the local branch/worktree for ${target}?\n\nOK = delete locally\nCancel = keep local files and branch`,
    );

    await api.deleteSession(id, deleteLocal);
    closeTab(id);
    load();
  };

  const runningSessions = sessions.filter((s) => s.status === "running");
  const stoppedSessions = sessions.filter((s) => s.status !== "running");

  // Tab workspace view
  if (openTabs.length > 0) {
    return (
      <div className="flex flex-col h-full">
        {/* Tab bar */}
        <div className="flex items-center border-b border-zinc-800 bg-zinc-900/50 px-1 overflow-x-auto">
          <button
            onClick={() => {
              setActiveTab(null);
              setOpenTabs([]);
            }}
            className="shrink-0 text-sm text-zinc-400 hover:text-white px-3 py-2 transition-colors"
          >
            &larr;
          </button>
          {openTabs.map((id) => {
            const session = sessions.find((s) => s.id === id);
            const isActive = activeTab === id;
            const isIdle = idleSessions.has(id);
            return (
              <div
                key={id}
                className={`flex items-center gap-1 shrink-0 border-r border-zinc-800 ${
                  isActive
                    ? "bg-zinc-950"
                    : "bg-zinc-900/50 hover:bg-zinc-800/50"
                }`}
              >
                <button
                  onClick={() => setActiveTab(id)}
                  className={`flex items-center gap-1.5 px-3 py-2 text-xs transition-colors ${
                    isActive ? "text-white" : "text-zinc-400"
                  }`}
                >
                  {isIdle && !isActive && (
                    <span className="w-2 h-2 rounded-full bg-amber-500 animate-pulse" />
                  )}
                  {session
                    ? `${session.repo_name}/${session.branch} (${session.cli_type})`
                    : id}
                </button>
                <button
                  onClick={(e) => {
                    e.stopPropagation();
                    closeTab(id);
                  }}
                  className="text-zinc-600 hover:text-zinc-300 pr-2 text-xs"
                >
                  &times;
                </button>
              </div>
            );
          })}
          <div className="flex-1" />
          {activeTab && (
            <button
              onClick={() => handleDelete(activeTab)}
              className="shrink-0 text-xs text-red-400 hover:text-red-300 px-2 py-1 mr-2"
            >
              Stop
            </button>
          )}
        </div>

        {/* Terminal area - all terminals stay mounted, CSS-hidden when inactive */}
        <div className="flex-1 relative">
          {openTabs.map((id) => (
            <div
              key={id}
              className="absolute inset-0 p-1"
              style={{ display: activeTab === id ? "block" : "none" }}
            >
              <Terminal
                sessionId={id}
                visible={activeTab === id}
                onIdleChange={(idle) => handleIdleChange(id, idle)}
              />
            </div>
          ))}
        </div>
      </div>
    );
  }

  // Session list view
  return (
    <div className="p-8 max-w-4xl">
      <div className="flex items-center justify-between mb-8">
        <div>
          <h2 className="text-2xl font-bold mb-1">Sessions</h2>
          <p className="text-zinc-400">Active and past coding sessions.</p>
        </div>
        <button
          onClick={() => setShowModal(true)}
          className="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white text-sm font-medium rounded-md transition-colors"
        >
          New Session
        </button>
      </div>

      {runningSessions.length > 0 && (
        <div className="mb-8">
          <h3 className="text-sm font-medium text-zinc-400 uppercase tracking-wider mb-3">
            Running
          </h3>
          <div className="grid gap-3 grid-cols-1 md:grid-cols-2">
            {runningSessions.map((s) => (
              <SessionCard
                key={s.id}
                session={s}
                onOpen={() => openTab(s.id)}
                onDelete={() => handleDelete(s.id)}
              />
            ))}
          </div>
        </div>
      )}

      {stoppedSessions.length > 0 && (
        <div>
          <h3 className="text-sm font-medium text-zinc-400 uppercase tracking-wider mb-3">
            Stopped
          </h3>
          <div className="grid gap-3 grid-cols-1 md:grid-cols-2">
            {stoppedSessions.map((s) => (
              <SessionCard
                key={s.id}
                session={s}
                onDelete={() => handleDelete(s.id)}
              />
            ))}
          </div>
        </div>
      )}

      {sessions.length === 0 && (
        <p className="text-zinc-500 text-sm">
          No sessions yet. Add a repo and create a session to start coding.
        </p>
      )}

      <NewSessionModal
        open={showModal}
        onClose={() => setShowModal(false)}
        onCreated={handleCreated}
      />
    </div>
  );
}

function SessionCard({
  session,
  onOpen,
  onDelete,
}: {
  session: SessionInfo;
  onOpen?: () => void;
  onDelete: () => void;
}) {
  const running = session.status === "running";
  return (
    <div className="p-4 rounded-lg border border-zinc-800 bg-zinc-900">
      <div className="flex items-start justify-between mb-2">
        <div>
          <p className="font-medium text-sm">
            {session.repo_owner}/{session.repo_name}
          </p>
          <p className="text-xs text-zinc-500">
            {session.branch} &middot; {session.cli_type} &middot; {session.id}
          </p>
        </div>
        <div
          className={`w-2.5 h-2.5 rounded-full mt-1 ${running ? "bg-emerald-500" : "bg-zinc-600"}`}
        />
      </div>
      <div className="flex gap-2 mt-3">
        {running && onOpen && (
          <button
            onClick={onOpen}
            className="text-xs bg-zinc-800 hover:bg-zinc-700 px-3 py-1 rounded transition-colors"
          >
            Open Terminal
          </button>
        )}
        <button
          onClick={onDelete}
          className="text-xs text-zinc-400 hover:text-red-400 px-2 py-1 rounded border border-zinc-700 hover:border-red-800 transition-colors"
        >
          {running ? "Stop" : "Remove"}
        </button>
      </div>
    </div>
  );
}
