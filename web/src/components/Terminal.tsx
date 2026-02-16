import { useEffect, useRef, useCallback, useState } from "react";
import type { IDisposable } from "@xterm/xterm";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import "@xterm/xterm/css/xterm.css";

interface TerminalProps {
  sessionId: string;
  visible?: boolean;
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

export default function Terminal({ sessionId, visible = true }: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const reconnectDelay = useRef(RECONNECT_DELAY);
  const disposed = useRef(false);
  const onDataDisposable = useRef<IDisposable | null>(null);
  const isTouch = useIsTouchDevice();

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
          try {
            term.write(new Uint8Array(e.data));
          } catch (err) {
            console.error("terminal write error:", err);
          }
        }
      };

      ws.onclose = (e) => {
        if (disposed.current) return;
        if (e.code === 1000) {
          try {
            term.write("\r\n\x1b[90m[Session ended]\x1b[0m\r\n");
          } catch (err) {
            console.error("terminal write error on session end:", err);
          }
          return;
        }
        // Reconnect
        try {
          term.write("\r\n\x1b[33m[Reconnecting...]\x1b[0m\r\n");
        } catch (err) {
          console.error("terminal write error on reconnect:", err);
        }
        reconnectTimer.current = setTimeout(() => {
          // Clear terminal before replay to avoid duplication
          try {
            term.clear();
          } catch (err) {
            console.error("terminal clear error:", err);
          }
          connect(term);
          reconnectDelay.current = Math.min(
            reconnectDelay.current * 2,
            MAX_RECONNECT_DELAY,
          );
        }, reconnectDelay.current);
      };

      ws.onerror = (ev) => {
        console.error("ws error for session", sessionId, ev);
      };

      // Dispose previous onData listener to prevent accumulation on reconnect
      onDataDisposable.current?.dispose();
      onDataDisposable.current = term.onData((data) => {
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

    // xterm.js v6 uses a custom scrollbar that doesn't handle wheel events,
    // so we bridge wheel events to the terminal's scroll API.
    const container = containerRef.current;
    const onWheel = (e: WheelEvent) => {
      if (e.deltaY === 0) return;
      e.preventDefault();
      const lineHeight = Math.ceil(term.options.fontSize! * 1.2);
      let lines: number;
      if (e.deltaMode === WheelEvent.DOM_DELTA_PIXEL) {
        lines =
          Math.sign(e.deltaY) *
          Math.max(1, Math.round(Math.abs(e.deltaY) / lineHeight));
      } else if (e.deltaMode === WheelEvent.DOM_DELTA_LINE) {
        lines = Math.round(e.deltaY);
      } else {
        lines = Math.sign(e.deltaY) * term.rows;
      }
      term.scrollLines(lines);
    };
    container.addEventListener("wheel", onWheel, { passive: false });

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
      container.removeEventListener("wheel", onWheel);
      onDataDisposable.current?.dispose();
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
