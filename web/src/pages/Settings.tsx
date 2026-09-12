import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../lib/api";
import { Card, Empty } from "../components/ui";

// Settings edits a working copy of the full config document (GET → mutate →
// POST). The daemon reports which changes need a restart (§12.4).
export default function SettingsPage() {
  const qc = useQueryClient();
  const { data } = useQuery({ queryKey: ["settings"], queryFn: api.settings, refetchInterval: false });
  const [draft, setDraft] = useState<Record<string, any> | null>(null);
  const [notice, setNotice] = useState<string>("");

  useEffect(() => {
    if (data && !draft) setDraft(structuredClone(data));
  }, [data, draft]);

  const save = useMutation({
    mutationFn: api.saveSettings,
    onSuccess: (res) => {
      qc.invalidateQueries({ queryKey: ["settings"] });
      setDraft(structuredClone(res.settings));
      setNotice(
        res.restart_required
          ? "Saved. Some changes need a daemon restart (systemctl restart routeviewnetd) to take effect."
          : "Saved and applied.",
      );
    },
    onError: (e) => setNotice(`Save failed: ${String(e)}`),
  });

  if (!draft) return <Empty text="Loading…" />;

  const set = (path: string[], value: any) => {
    setDraft((d) => {
      const next = structuredClone(d)!;
      let obj = next;
      for (const key of path.slice(0, -1)) obj = obj[key];
      obj[path[path.length - 1]] = value;
      return next;
    });
  };

  const num = (path: string[], label: string, hint?: string) => (
    <label className="flex items-center justify-between gap-4 text-sm">
      <span>
        {label}
        {hint && (
          <span className="block text-xs" style={{ color: "var(--ink-muted)" }}>
            {hint}
          </span>
        )}
      </span>
      <input
        type="number"
        value={path.reduce((o: any, k) => o[k], draft)}
        onChange={(e) => set(path, Number(e.target.value))}
        className="w-24 rounded border bg-transparent px-2 py-1 text-right"
        style={{ borderColor: "var(--border)" }}
      />
    </label>
  );

  const toggle = (path: string[], label: string, hint?: string) => (
    <label className="flex items-center justify-between gap-4 text-sm">
      <span>
        {label}
        {hint && (
          <span className="block text-xs" style={{ color: "var(--ink-muted)" }}>
            {hint}
          </span>
        )}
      </span>
      <input
        type="checkbox"
        checked={path.reduce((o: any, k) => o[k], draft)}
        onChange={(e) => set(path, e.target.checked)}
      />
    </label>
  );

  const list = (path: string[], label: string) => (
    <label className="flex flex-col gap-1 text-sm">
      {label}
      <input
        // A list setting that was never set can arrive as null; calling
        // .join on it would blank the whole Settings page.
        value={((path.reduce((o: any, k) => o[k], draft) as string[] | null) ?? []).join(", ")}
        onChange={(e) =>
          set(
            path,
            e.target.value
              .split(",")
              .map((s) => s.trim())
              .filter(Boolean),
          )
        }
        className="rounded border bg-transparent px-2 py-1"
        style={{ borderColor: "var(--border)" }}
      />
    </label>
  );

  const lanEnabled = draft.server.host === "0.0.0.0";

  return (
    <div className="flex max-w-2xl flex-col gap-4">
      <h1 className="text-xl font-semibold">Settings</h1>

      <Card title="Collection">
        <div className="flex flex-col gap-3">
          {num(["collection", "interval_seconds"], "Network check interval (s)")}
          {num(["collection", "memory_interval_seconds"], "Memory interval (s)")}
          {num(["collection", "process_memory_interval_seconds"], "Process scan interval (s)")}
          {num(["collection", "load_interval_seconds"], "Load average interval (s)")}
          {num(["collection", "app_traffic_interval_seconds"], "App data usage interval (s)")}
          {num(["storage", "retention_days"], "Data retention (days)")}
          {toggle(["collection", "enable_device_discovery"], "Device discovery")}
          {toggle(["collection", "enable_dns_checks"], "DNS checks")}
          {toggle(["collection", "enable_latency_checks"], "Latency checks")}
          {toggle(["collection", "enable_memory_monitoring"], "Memory monitoring")}
          {toggle(["collection", "enable_process_memory_monitoring"], "Process memory monitoring")}
          {toggle(["collection", "enable_load_monitoring"], "Load monitoring")}
          {toggle(["collection", "enable_app_traffic_monitoring"], "App data usage")}
        </div>
      </Card>

      <Card title="Checks">
        <div className="flex flex-col gap-3">
          {list(["checks", "internet_targets"], "Internet targets (comma-separated)")}
          {list(["checks", "dns_domains"], "DNS check domains")}
          {toggle(["checks", "dns_check_aaaa"], "Also check AAAA (IPv6) records")}
          <label className="flex items-center justify-between gap-4 text-sm">
            Primary interface override
            <input
              value={draft.checks.primary_interface_override}
              placeholder="auto"
              onChange={(e) => set(["checks", "primary_interface_override"], e.target.value)}
              className="w-32 rounded border bg-transparent px-2 py-1"
              style={{ borderColor: "var(--border)" }}
            />
          </label>
        </div>
      </Card>

      <Card title="Alert thresholds">
        <div className="flex flex-col gap-3">
          {num(["alerts", "packet_loss_warning"], "Packet loss warning (%)")}
          {num(["alerts", "dns_latency_warning_ms"], "DNS latency warning (ms)")}
          {num(["alerts", "memory_usage_warning_percent"], "Memory usage warning (%)")}
          {num(["alerts", "swap_usage_warning_percent"], "Swap usage warning (%)")}
          {num(["alerts", "daemon_memory_warning_mb"], "Daemon memory warning (MB)")}
          {num(["alerts", "load_avg_multiplier_warning"], "Load warning (× cores)")}
          {num(["alerts", "trigger_debounce_checks"], "Trigger debounce", "consecutive failing checks before an alert opens")}
          {num(["alerts", "resolve_debounce_checks"], "Resolve debounce", "consecutive healthy checks before it resolves")}
        </div>
      </Card>

      <Card title="Privacy & access">
        <div className="flex flex-col gap-3">
          {toggle(["privacy", "telemetry"], "Telemetry", "off by default; nothing leaves this machine")}
          {toggle(
            ["privacy", "store_process_command"],
            "Store process command lines",
            "may capture secrets/tokens passed as arguments; off by default",
          )}
          {toggle(["devices", "resolve_hostnames"], "Reverse-DNS device hostnames")}
          <label className="flex items-center justify-between gap-4 text-sm">
            <span>
              LAN access (bind 0.0.0.0)
              <span className="block text-xs" style={{ color: "var(--status-warning)" }}>
                ⚠ exposes the dashboard and API to your network with no authentication
              </span>
            </span>
            <input
              type="checkbox"
              checked={lanEnabled}
              onChange={(e) => set(["server", "host"], e.target.checked ? "0.0.0.0" : "127.0.0.1")}
            />
          </label>
        </div>
      </Card>

      <div className="flex items-center gap-3">
        <button
          onClick={() => save.mutate(draft)}
          disabled={save.isPending}
          className="rounded-lg px-4 py-2 text-sm font-medium"
          style={{ background: "var(--series-1)", color: "#fff" }}
        >
          {save.isPending ? "Saving…" : "Save settings"}
        </button>
        {notice && (
          <span className="text-sm" style={{ color: "var(--ink-secondary)" }}>
            {notice}
          </span>
        )}
      </div>
    </div>
  );
}
