import { useEffect, useState } from "react";
import { NavLink, Outlet, useLocation } from "react-router-dom";
import { useConnectionStatus } from "../hooks/useConnectionStatus";

const navItems = [
  {
    to: "/",
    label: "Dashboard",
    icon: "M3 12l2-2m0 0l7-7 7 7M5 10v10a1 1 0 001 1h3m10-11l2 2m-2-2v10a1 1 0 01-1 1h-3m-6 0a1 1 0 001-1v-4a1 1 0 011-1h2a1 1 0 011 1v4a1 1 0 001 1m-6 0h6",
  },
  {
    to: "/repos",
    label: "Repositories",
    icon: "M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z",
  },
  {
    to: "/sessions",
    label: "Sessions",
    icon: "M8 9l3 3-3 3m5 0h3M5 20h14a2 2 0 002-2V6a2 2 0 00-2-2H5a2 2 0 00-2 2v12a2 2 0 002 2z",
  },
  {
    to: "/settings",
    label: "Settings",
    icon: "M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z M15 12a3 3 0 11-6 0 3 3 0 016 0z",
  },
];

export default function Layout() {
  const [isNavOpen, setIsNavOpen] = useState(false);
  const location = useLocation();
  const connectionStatus = useConnectionStatus();

  useEffect(() => {
    setIsNavOpen(false);
  }, [location.pathname]);

  return (
    <div className="flex h-dvh bg-zinc-950 text-zinc-100">
      <button
        type="button"
        aria-label="Close navigation"
        onClick={() => setIsNavOpen(false)}
        className={`fixed inset-0 z-30 bg-black/60 transition-opacity md:hidden ${
          isNavOpen ? "opacity-100" : "pointer-events-none opacity-0"
        }`}
      />

      <aside
        className={`fixed inset-y-0 left-0 z-40 w-56 border-r border-zinc-800 bg-zinc-950 flex flex-col transform transition-transform duration-200 ease-out md:static md:translate-x-0 ${
          isNavOpen ? "translate-x-0" : "-translate-x-full"
        }`}
      >
        <div className="p-4 border-b border-zinc-800">
          <div className="flex items-start justify-between gap-3">
            <div>
              <h1 className="text-lg font-bold tracking-tight">
                Superposition
              </h1>
              <p className="text-xs text-zinc-500">AI Coding Sessions</p>
            </div>
            <button
              type="button"
              onClick={() => setIsNavOpen(false)}
              className="md:hidden rounded p-1 text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800/60 transition-colors"
              aria-label="Close menu"
            >
              <svg
                className="w-5 h-5"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M6 18L18 6M6 6l12 12"
                />
              </svg>
            </button>
          </div>
        </div>
        <nav className="flex-1 p-2 space-y-1">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === "/"}
              onClick={() => setIsNavOpen(false)}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2 rounded-md text-sm transition-colors ${
                  isActive
                    ? "bg-zinc-800 text-white"
                    : "text-zinc-400 hover:text-zinc-200 hover:bg-zinc-800/50"
                }`
              }
            >
              <svg
                className="w-4 h-4 shrink-0"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                strokeWidth={2}
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d={item.icon}
                />
              </svg>
              {item.label}
            </NavLink>
          ))}
        </nav>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="md:hidden flex items-center gap-3 px-4 py-3 border-b border-zinc-800">
          <button
            type="button"
            onClick={() => setIsNavOpen(true)}
            className="rounded p-1 text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800/60 transition-colors"
            aria-label="Open menu"
          >
            <svg
              className="w-6 h-6"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              strokeWidth={2}
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M4 6h16M4 12h16M4 18h16"
              />
            </svg>
          </button>
          <div>
            <h1 className="text-sm font-semibold tracking-wide">
              Superposition
            </h1>
            <p className="text-[11px] leading-tight text-zinc-500">
              AI Coding Sessions
            </p>
          </div>
        </header>

        {connectionStatus === "offline" && (
          <div className="px-4 py-2 bg-amber-600/20 border-b border-amber-700/50 text-amber-300 text-sm text-center">
            Superposition is offline — your laptop may be asleep or
            disconnected.
          </div>
        )}

        <main className="flex-1 overflow-auto">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
