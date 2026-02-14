import { useEffect, useRef, useCallback, useState } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import "@xterm/xterm/css/xterm.css";

interface TerminalProps {
  sessionId: string;
  visible?: boolean;
  onIdleChange?: (idle: boolean) => void;
}

const RECONNECT_DELAY = 1000;
const MAX_RECONNECT_DELAY = 10000;

// ANSI escape sequences for special keys
const KEY_SEQUENCES: Record<string, string> = {
  up: "\x1b[A",
  down: "\x1b[B",
  left: "\x1b[D",
  right: "\x1b[C",
  enter: "\r",
  tab: "\t",
  "shift-tab": "\x1b[Z",
  escape: "\x1b",
  "ctrl-c": "\x03",
  backspace: "\x7f",
};

function useIsTouchDevice() {
  const [isTouch, setIsTouch] = useState(false);
  useEffect(() => {
    const check = () =>
      setIsTouch("ontouchstart" in window || navigator.maxTouchPoints > 0);
    check();
    window.addEventListener("resize", check);
    return () => window.removeEventListener("resize", check);
  }, []);
  return isTouch;
}

interface VirtualKeybarProps {
  onKey: (seq: string) => void;
}

function VirtualKeybar({ onKey }: VirtualKeybarProps) {
  const [showExtra, setShowExtra] = useState(false);

  const btn = (label: string, key: string, className?: string) => (
    <button
      type="button"
      onPointerDown={(e) => {
        e.preventDefault();
        onKey(KEY_SEQUENCES[key]);
      }}
      className={`flex items-center justify-center rounded-md bg-zinc-800 border border-zinc-700
        active:bg-zinc-600 text-sm font-medium select-none touch-manipulation ${className || "h-10 min-w-[2.75rem] px-2"}`}
    >
      {label}
    </button>
  );

  return (
    <div className="flex-shrink-0 bg-zinc-900 border-t border-zinc-800 px-2 py-1.5 safe-area-pb">
      <div className="overflow-x-auto pb-0.5">
        <div className="flex items-center gap-1.5 min-w-max">
          {/* Arrow keys */}
          {btn("←", "left")}
          {btn("↓", "down")}
          {btn("↑", "up")}
          {btn("→", "right")}

          <div className="w-px h-6 bg-zinc-700 mx-0.5" />

          {/* Common keys */}
          {btn("Enter", "enter", "h-10 px-3")}
          {btn("Tab", "tab", "h-10 px-3")}
          {btn("⇧Tab", "shift-tab", "h-10 px-3")}

          <div className="flex-1" />

          {/* Toggle extra keys */}
          <button
            type="button"
            onPointerDown={(e) => {
              e.preventDefault();
              setShowExtra((current) => !current);
            }}
            className={`flex items-center justify-center rounded-md border text-sm font-medium
              select-none touch-manipulation h-10 px-2 ${
                showExtra
                  ? "bg-zinc-600 border-zinc-500"
                  : "bg-zinc-800 border-zinc-700 active:bg-zinc-600"
              }`}
          >
            ···
          </button>
        </div>
      </div>

      {showExtra && (
        <div className="flex items-center gap-1.5 mt-1.5 overflow-x-auto">
          {btn("Esc", "escape")}
          {btn("^C", "ctrl-c")}
          {btn("⌫", "backspace")}
        </div>
      )}
    </div>
  );
}

export default function Terminal({
  sessionId,
  visible = true,
  onIdleChange,
}: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const reconnectDelay = useRef(RECONNECT_DELAY);
  const disposed = useRef(false);
  const isTouch = useIsTouchDevice();

  // Idle detection refs
  const idleTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const settled = useRef(false);
  const settleTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const sessionEnded = useRef(false);
  const onIdleChangeRef = useRef(onIdleChange);
  onIdleChangeRef.current = onIdleChange;

  const sendInput = useCallback((data: string) => {
    const ws = wsRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(new TextEncoder().encode(data));
    }
  }, []);

  const connect = useCallback(
    (term: XTerm) => {
      if (disposed.current) return;

      const proto = location.protocol === "https:" ? "wss:" : "ws:";
      const ws = new WebSocket(
        `${proto}//${location.host}/ws/session/${sessionId}`,
      );
      ws.binaryType = "arraybuffer";
      wsRef.current = ws;

      ws.onopen = () => {
        reconnectDelay.current = RECONNECT_DELAY;
        ws.send(
          JSON.stringify({
            type: "resize",
            data: { rows: term.rows, cols: term.cols },
          }),
        );
      };

      ws.onmessage = (e) => {
        if (e.data instanceof ArrayBuffer) {
          term.write(new Uint8Array(e.data));
        }

        // Idle detection: wait for settle period after first message (replay buffer burst)
        if (!settled.current && !settleTimer.current) {
          settleTimer.current = setTimeout(() => {
            settled.current = true;
          }, 2000);
        }

        if (settled.current && !sessionEnded.current) {
          clearTimeout(idleTimer.current);
          onIdleChangeRef.current?.(false);
          idleTimer.current = setTimeout(() => {
            onIdleChangeRef.current?.(true);
          }, 5000);
        }
      };

      ws.onclose = (e) => {
        // Clear idle detection timers
        clearTimeout(idleTimer.current);
        clearTimeout(settleTimer.current);
        settled.current = false;
        settleTimer.current = undefined;

        if (disposed.current) return;
        if (e.code === 1000) {
          sessionEnded.current = true;
          onIdleChangeRef.current?.(true);
          term.write("\r\n\x1b[90m[Session ended]\x1b[0m\r\n");
          return;
        }
        // Clear idle state during reconnect
        onIdleChangeRef.current?.(false);
        // Reconnect
        term.write("\r\n\x1b[33m[Reconnecting...]\x1b[0m\r\n");
        reconnectTimer.current = setTimeout(() => {
          // Clear terminal before replay to avoid duplication
          term.clear();
          connect(term);
          reconnectDelay.current = Math.min(
            reconnectDelay.current * 2,
            MAX_RECONNECT_DELAY,
          );
        }, reconnectDelay.current);
      };

      ws.onerror = () => {};

      term.onData((data) => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(new TextEncoder().encode(data));
        }
      });
    },
    [sessionId],
  );

  useEffect(() => {
    if (!containerRef.current) return;
    disposed.current = false;

    const term = new XTerm({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
      theme: {
        background: "#09090b",
        foreground: "#fafafa",
        cursor: "#fafafa",
        selectionBackground: "#3f3f46",
      },
      allowProposedApi: true,
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.loadAddon(new WebLinksAddon());

    term.open(containerRef.current);
    fitAddon.fit();

    termRef.current = term;
    fitRef.current = fitAddon;

    // Resize handling
    const observer = new ResizeObserver(() => {
      fitAddon.fit();
    });
    observer.observe(containerRef.current);

    term.onResize(({ rows, cols }) => {
      const ws = wsRef.current;
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ type: "resize", data: { rows, cols } }));
      }
    });

    connect(term);

    return () => {
      disposed.current = true;
      clearTimeout(reconnectTimer.current);
      clearTimeout(idleTimer.current);
      clearTimeout(settleTimer.current);
      observer.disconnect();
      wsRef.current?.close();
      term.dispose();
    };
  }, [sessionId, connect]);

  // Re-fit when visibility changes
  useEffect(() => {
    if (visible && fitRef.current) {
      setTimeout(() => fitRef.current?.fit(), 50);
    }
  }, [visible]);

  return (
    <div
      className="h-full flex flex-col"
      style={{ display: visible ? "flex" : "none" }}
    >
      <div ref={containerRef} className="flex-1 min-h-0" />
      {isTouch && <VirtualKeybar onKey={sendInput} />}
    </div>
  );
}
