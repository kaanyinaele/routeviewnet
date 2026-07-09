import { NavLink, Route, Routes } from "react-router-dom";
import { useLive } from "./lib/useLive";
import OverviewPage from "./pages/Overview";
import TrafficPage from "./pages/Traffic";
import DevicesPage from "./pages/Devices";
import InternetPage from "./pages/Internet";
import SystemPage from "./pages/System";
import AlertsPage from "./pages/Alerts";
import TroubleshootPage from "./pages/Troubleshoot";
import SettingsPage from "./pages/Settings";

const nav = [
  { to: "/", label: "Overview" },
  { to: "/traffic", label: "Traffic" },
  { to: "/devices", label: "Devices" },
  { to: "/internet", label: "Internet Health" },
  { to: "/system", label: "System" },
  { to: "/alerts", label: "Alerts" },
  { to: "/troubleshoot", label: "Troubleshooting" },
  { to: "/settings", label: "Settings" },
];

export default function App() {
  const connected = useLive();
  return (
    <div className="flex min-h-screen">
      <aside className="w-56 shrink-0 border-r border-[var(--border)] p-4 flex flex-col gap-1">
        <div className="mb-4 px-2">
          <div className="text-lg font-semibold">RouteViewNet</div>
          <div className="text-xs" style={{ color: "var(--ink-muted)" }}>
            Local Network &amp; Memory Health Monitor
          </div>
        </div>
        {nav.map((n) => (
          <NavLink
            key={n.to}
            to={n.to}
            end={n.to === "/"}
            className={({ isActive }) =>
              `rounded-lg px-3 py-2 text-sm transition-colors ${
                isActive
                  ? "bg-[var(--surface-1)] text-[var(--ink-primary)] border border-[var(--border)]"
                  : "text-[var(--ink-secondary)] hover:text-[var(--ink-primary)]"
              }`
            }
          >
            {n.label}
          </NavLink>
        ))}
        <div className="mt-auto flex items-center gap-2 px-3 py-2 text-xs" style={{ color: "var(--ink-muted)" }}>
          <span
            aria-hidden
            className="inline-block h-2 w-2 rounded-full"
            style={{ background: connected ? "var(--status-good)" : "var(--ink-muted)" }}
          />
          {connected ? "live" : "reconnecting…"}
        </div>
      </aside>
      <main className="flex-1 p-6 max-w-6xl">
        <Routes>
          <Route path="/" element={<OverviewPage />} />
          <Route path="/traffic" element={<TrafficPage />} />
          <Route path="/devices" element={<DevicesPage />} />
          <Route path="/internet" element={<InternetPage />} />
          <Route path="/system" element={<SystemPage />} />
          <Route path="/alerts" element={<AlertsPage />} />
          <Route path="/troubleshoot" element={<TroubleshootPage />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Routes>
      </main>
    </div>
  );
}
