import { useEffect, useRef } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { api } from "../lib/api";

const POLL_INTERVAL = 5000;

interface TerminalPreviewProps {
  sessionId: string;
}

export default function TerminalPreview({ sessionId }: TerminalPreviewProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);

  useEffect(() => {
    if (!containerRef.current) return;

    const term = new XTerm({
      fontSize: 10,
      fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', monospace",
      theme: {
        background: "#09090b",
        foreground: "#fafafa",
        cursor: "#09090b",
        selectionBackground: "#3f3f46",
      },
      cursorBlink: false,
      cursorInactiveStyle: "none",
      disableStdin: true,
      scrollback: 5000,
      allowProposedApi: true,
    });

    term.open(containerRef.current);
    termRef.current = term;

    let disposed = false;

    const fetchReplay = () => {
      if (disposed) return;
      api
        .getSessionReplay(sessionId)
        .then((buf) => {
          if (disposed || buf.byteLength === 0) return;
          term.reset();
          term.write(new Uint8Array(buf), () => {
            term.scrollToBottom();
          });
        })
        .catch(() => {});
    };

    fetchReplay();
    const interval = setInterval(fetchReplay, POLL_INTERVAL);

    return () => {
      disposed = true;
      clearInterval(interval);
      term.dispose();
      termRef.current = null;
    };
  }, [sessionId]);

  return (
    <div
      ref={containerRef}
      className="w-full h-[150px] rounded overflow-hidden pointer-events-none"
    />
  );
}
