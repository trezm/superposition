import { useEffect, useState } from "react";
import { api, SuperpositionOfflineError } from "../lib/api";

interface CLIStatus {
  name: string;
  installed: boolean;
  authed: boolean;
  path?: string;
}

interface Health {
  status: string;
  clis: CLIStatus[];
  git: boolean;
}

export default function Dashboard() {
  const [health, setHealth] = useState<Health | null>(null);
  const [offline, setOffline] = useState(false);

  useEffect(() => {
    api
      .health()
      .then(setHealth)
      .catch((e) => {
        if (e instanceof SuperpositionOfflineError) {
          setOffline(true);
        } else {
          console.error(e);
        }
      });
  }, []);

  return (
    <div className="w-full max-w-3xl p-4 sm:p-6 lg:p-8">
      <h2 className="text-xl sm:text-2xl font-bold mb-1">Dashboard</h2>
      <p className="text-sm sm:text-base text-zinc-400 mb-6 sm:mb-8">
        Run Claude Code and Codex sessions against your GitHub repos.
      </p>

      {offline && (
        <p className="text-sm text-zinc-500">
          System status unavailable while superposition is offline.
        </p>
      )}

      {health && (
        <div className="space-y-4">
          <h3 className="text-sm font-medium text-zinc-400 uppercase tracking-wider">
            System Status
          </h3>
          <div className="grid gap-3">
            <StatusCard
              label="Git"
              ok={health.git}
              detail={health.git ? "Installed" : "Not found"}
            />
            {health.clis.map((cli) => (
              <StatusCard
                key={cli.name}
                label={cli.name}
                ok={cli.installed}
                detail={
                  !cli.installed ? "Not installed" : cli.path || "Installed"
                }
              />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

function StatusCard({
  label,
  ok,
  detail,
}: {
  label: string;
  ok: boolean;
  detail: string;
}) {
  return (
    <div className="flex items-start gap-3 sm:items-center justify-between p-4 rounded-lg border border-zinc-800 bg-zinc-900">
      <div>
        <p className="font-medium capitalize">{label}</p>
        <p className="text-sm text-zinc-500">{detail}</p>
      </div>
      <div
        className={`w-3 h-3 rounded-full mt-1 sm:mt-0 ${ok ? "bg-emerald-500" : "bg-amber-500"}`}
      />
    </div>
  );
}
