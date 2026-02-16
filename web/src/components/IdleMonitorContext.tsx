import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useState,
  useCallback,
  type ReactNode,
} from "react";
import { api } from "../lib/api";

interface IdleMonitorContextValue {
  idleSessions: Set<string>;
}

const IdleMonitorContext = createContext<IdleMonitorContextValue>({
  idleSessions: new Set(),
});

export function useIdleMonitor() {
  return useContext(IdleMonitorContext);
}

interface SessionInfo {
  id: string;
  status: string;
  repo_name: string;
  branch: string;
}

const SETTLE_MS = 2000;
const IDLE_MS = 5000;
const POLL_MS = 10_000;
const RECONNECT_MS = 3000;
const MAX_RECONNECT_MS = 30_000;

interface SessionMonitor {
  ws: WebSocket;
  settleTimer: ReturnType<typeof setTimeout> | undefined;
  idleTimer: ReturnType<typeof setTimeout> | undefined;
  settled: boolean;
  reconnectDelay: number;
  ended: boolean;
}

export function IdleMonitorProvider({ children }: { children: ReactNode }) {
  const [sessions, setSessions] = useState<SessionInfo[]>([]);
  const [idleSessions, setIdleSessions] = useState<Set<string>>(new Set());
  const notifiedSessions = useRef<Set<string>>(new Set());
  const monitors = useRef<Map<string, SessionMonitor>>(new Map());
  const disposed = useRef(false);

  // Poll sessions
  useEffect(() => {
    disposed.current = false;
    let timeout: ReturnType<typeof setTimeout>;
    let cancelled = false;

    async function poll() {
      try {
        const data = await api.getSessions();
        if (!cancelled) setSessions(data);
      } catch {
        // ignore
      }
      if (!cancelled) timeout = setTimeout(poll, POLL_MS);
    }

    poll();
    return () => {
      cancelled = true;
      clearTimeout(timeout);
    };
  }, []);

  const markIdle = useCallback((sessionId: string) => {
    setIdleSessions((prev) => {
      if (prev.has(sessionId)) return prev;
      const next = new Set(prev);
      next.add(sessionId);
      return next;
    });
  }, []);

  const clearIdle = useCallback((sessionId: string) => {
    setIdleSessions((prev) => {
      if (!prev.has(sessionId)) return prev;
      const next = new Set(prev);
      next.delete(sessionId);
      return next;
    });
    notifiedSessions.current.delete(sessionId);
  }, []);

  // Manage WS connections for running sessions
  useEffect(() => {
    const runningSessions = sessions.filter((s) => s.status === "running");
    const runningIds = new Set(runningSessions.map((s) => s.id));

    // Close monitors for sessions that are no longer running
    for (const [id, monitor] of monitors.current) {
      if (!runningIds.has(id)) {
        clearTimeout(monitor.settleTimer);
        clearTimeout(monitor.idleTimer);
        monitor.ws.close();
        monitors.current.delete(id);
        // Don't clear idle — stopped sessions can stay idle in the set
        // They'll be cleaned up when the user interacts
      }
    }

    // Open monitors for new running sessions
    for (const session of runningSessions) {
      if (monitors.current.has(session.id)) continue;
      openMonitor(session.id);
    }
  }, [sessions, clearIdle]);

  function openMonitor(sessionId: string) {
    if (disposed.current) return;

    const proto = location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(
      `${proto}//${location.host}/ws/session/${sessionId}`,
    );
    ws.binaryType = "arraybuffer";

    const monitor: SessionMonitor = {
      ws,
      settleTimer: undefined,
      idleTimer: undefined,
      settled: false,
      reconnectDelay: RECONNECT_MS,
      ended: false,
    };

    monitors.current.set(sessionId, monitor);

    ws.onmessage = () => {
      // Start settle timer after first message (replay buffer burst)
      if (!monitor.settled && !monitor.settleTimer) {
        monitor.settleTimer = setTimeout(() => {
          monitor.settled = true;
        }, SETTLE_MS);
      }

      if (monitor.settled && !monitor.ended) {
        clearTimeout(monitor.idleTimer);
        clearIdle(sessionId);
        monitor.idleTimer = setTimeout(() => {
          markIdle(sessionId);
        }, IDLE_MS);
      }
    };

    ws.onclose = (e) => {
      clearTimeout(monitor.idleTimer);
      clearTimeout(monitor.settleTimer);

      monitors.current.delete(sessionId);

      if (disposed.current) return;

      if (e.code === 1000) {
        // Session ended normally
        monitor.ended = true;
        markIdle(sessionId);
        return;
      }

      // Abnormal close — reconnect with backoff
      clearIdle(sessionId);
      const delay = monitor.reconnectDelay;
      setTimeout(() => {
        if (disposed.current) return;
        openMonitor(sessionId);
      }, delay);
      monitor.reconnectDelay = Math.min(
        monitor.reconnectDelay * 2,
        MAX_RECONNECT_MS,
      );
    };

    ws.onerror = () => {
      // onclose will fire after this
    };
  }

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      disposed.current = true;
      for (const [, monitor] of monitors.current) {
        clearTimeout(monitor.settleTimer);
        clearTimeout(monitor.idleTimer);
        monitor.ws.close();
      }
      monitors.current.clear();
    };
  }, []);

  // Browser tab title
  useEffect(() => {
    document.title =
      idleSessions.size > 0 ? "(!) Superposition" : "Superposition";
  }, [idleSessions]);

  // OS notifications for idle sessions
  useEffect(() => {
    if (
      typeof Notification === "undefined" ||
      Notification.permission !== "granted"
    )
      return;

    for (const sessionId of idleSessions) {
      if (notifiedSessions.current.has(sessionId)) continue;
      notifiedSessions.current.add(sessionId);

      const session = sessions.find((s) => s.id === sessionId);
      const label = session
        ? `${session.repo_name}/${session.branch}`
        : sessionId;

      navigator.serviceWorker?.ready
        .then((reg) =>
          reg.showNotification("Superposition", {
            body: `${label} needs your attention`,
            tag: `idle-${sessionId}`,
            data: { sessionId },
          }),
        )
        .catch((err) => console.error("Failed to send notification:", err));
    }
  }, [idleSessions, sessions]);

  return (
    <IdleMonitorContext.Provider value={{ idleSessions }}>
      {children}
    </IdleMonitorContext.Provider>
  );
}
