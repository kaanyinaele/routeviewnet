import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../lib/api";
import { Card, PageState } from "../components/ui";
import { Icon } from "../components/icons";
import { Badge, NumberField, Row, Subhead, Switch, TextField } from "../components/form";
import {
  notifyPermission,
  notifyPreferred,
  notifySupport,
  requestNotifyPermission,
  setNotifyPreferred,
} from "../lib/notify";

// Settings edits a working copy of the full config document (GET → mutate →
// POST). The daemon reports which changes need a restart (§12.4).
export default function SettingsPage() {
  const qc = useQueryClient();
  const { data, error, isLoading } = useQuery({
    queryKey: ["settings"],
    queryFn: api.settings,
    refetchInterval: false,
  });
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
          ? "Saved. Restart the daemon (systemctl restart routeviewnetd) for the marked settings to take effect."
          : "Saved and applied.",
      );
    },
    onError: (e) => setNotice(`Save failed: ${String(e)}`),
  });

  // Browser-notification state lives in the browser, not the config document:
  // permission is granted per browser, so there is nothing for the daemon to
  // store. Re-read after every change rather than mirroring it in state.
  const [notify, setNotify] = useState(() => ({
    support: notifySupport(),
    permission: notifyPermission(),
    preferred: notifyPreferred(),
  }));
  const refreshNotify = () =>
    setNotify({ support: notifySupport(), permission: notifyPermission(), preferred: notifyPreferred() });

  const setBrowserNotifications = async (on: boolean) => {
    if (!on) {
      setNotifyPreferred(false);
      refreshNotify();
      return;
    }
    // requestPermission must be called from the click, not on page load —
    // browsers reject unprompted requests and some show nothing at all.
    const permission = await requestNotifyPermission();
    setNotifyPreferred(permission === "granted");
    refreshNotify();
  };

  const [testNotice, setTestNotice] = useState("");
  const test = useMutation({
    mutationFn: api.testNotification,
    onSuccess: (res) =>
      setTestNotice(
        res.webhook_configured
          ? "Sent. It should appear on this page's live feed and at your webhook URL."
          : "Sent to the dashboard. No webhook URL is saved, so nothing was posted.",
      ),
    onError: (e) => setTestNotice(`Test failed: ${String(e)}`),
  });

  // Settings is the one page where failing silently is worst: a blank form
  // looks like "you have no settings" rather than "we could not read them".
  if (!draft) {
    return (
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
        <h1 className="text-xl font-semibold">Settings</h1>
        <Card>
          <PageState error={error} isLoading={isLoading || !error} />
        </Card>
      </div>
    );
  }

  const set = (path: string[], value: any) => {
    // A save confirmation next to fields you have since edited is misleading.
    setNotice("");
    setDraft((d) => {
      const next = structuredClone(d)!;
      let obj = next;
      for (const key of path.slice(0, -1)) obj = obj[key];
      obj[path[path.length - 1]] = value;
      return next;
    });
  };

  const at = (path: string[]) => path.reduce((o: any, k) => o[k], draft);

  const num = (path: string[], label: string, unit?: string, hint?: string, width?: string) => (
    <Row label={label} hint={hint}>
      <NumberField value={at(path)} unit={unit} width={width} onChange={(n) => set(path, n)} />
    </Row>
  );

  const toggle = (path: string[], label: string, hint?: string) => (
    <Row label={label} hint={hint}>
      <Switch checked={at(path)} onChange={(on) => set(path, on)} />
    </Row>
  );

  const text = (path: string[], label: string, placeholder?: string, hint?: string) => (
    <Row label={label} hint={hint} stacked>
      <TextField value={(at(path) as string | null) ?? ""} placeholder={placeholder} onChange={(s) => set(path, s)} />
    </Row>
  );

  const list = (path: string[], label: string, hint?: string) => (
    <Row label={label} hint={hint} stacked>
      <TextField
        mono
        // A list setting that was never set can arrive as null; calling
        // .join on it would blank the whole Settings page.
        value={((at(path) as string[] | null) ?? []).join(", ")}
        onChange={(s) =>
          set(
            path,
            s
              .split(",")
              .map((v) => v.trim())
              .filter(Boolean),
          )
        }
      />
    </Row>
  );

  const lanEnabled = draft.server.host === "0.0.0.0";
  const changes = data ? countChanges(data, draft) : 0;

  return (
    // The shell drops its width cap for this route (App.tsx), so centring here
    // centres against the full column beside the sidebar.
    <div className="mx-auto flex w-full max-w-2xl flex-col gap-4 pb-4">
      <h1 className="text-xl font-semibold">Settings</h1>

      <Card
        title={
          <>
            <Icon name="activity" size={15} />
            Collection
          </>
        }
      >
        <div className="flex flex-col gap-3">
          <Subhead icon="clock">How often to sample</Subhead>
          {num(["collection", "interval_seconds"], "Network checks", "s")}
          {num(["collection", "memory_interval_seconds"], "Memory", "s")}
          {num(["collection", "process_memory_interval_seconds"], "Process scan", "s")}
          {num(["collection", "load_interval_seconds"], "Load average", "s")}
          {num(["collection", "app_traffic_interval_seconds"], "App data usage", "s")}
          {num(["storage", "retention_days"], "Keep history for", "days")}

          <Subhead icon="eye">What to monitor</Subhead>
          {toggle(["collection", "enable_device_discovery"], "Device discovery")}
          {toggle(["collection", "enable_dns_checks"], "DNS checks")}
          {toggle(["collection", "enable_latency_checks"], "Latency checks")}
          {toggle(["collection", "enable_memory_monitoring"], "Memory monitoring")}
          {toggle(["collection", "enable_process_memory_monitoring"], "Process memory monitoring")}
          {toggle(["collection", "enable_load_monitoring"], "Load monitoring")}
          {toggle(["collection", "enable_app_traffic_monitoring"], "App data usage")}
        </div>
      </Card>

      <Card
        title={
          <>
            <Icon name="globe" size={15} />
            Checks
          </>
        }
      >
        <div className="flex flex-col gap-3">
          {list(["checks", "internet_targets"], "Internet targets", "Addresses pinged to tell an internet outage from a local one. Comma-separated.")}
          {list(["checks", "dns_domains"], "DNS check domains", "Resolved on each cycle to measure DNS health. Comma-separated.")}
          {toggle(["checks", "dns_check_aaaa"], "Also check AAAA (IPv6) records")}
          <Row
            label="Primary interface override"
            hint="Leave empty to detect the active interface automatically."
            stacked
          >
            <TextField
              mono
              value={draft.checks.primary_interface_override}
              placeholder="auto"
              onChange={(s) => set(["checks", "primary_interface_override"], s)}
            />
          </Row>
        </div>
      </Card>

      <Card
        title={
          <>
            <Icon name="gauge" size={15} />
            Alert thresholds
          </>
        }
      >
        <div className="flex flex-col gap-3">
          {num(["alerts", "packet_loss_warning"], "Packet loss warning", "%")}
          {num(["alerts", "dns_latency_warning_ms"], "DNS latency warning", "ms")}
          {num(["alerts", "memory_usage_warning_percent"], "Memory usage warning", "%")}
          {num(["alerts", "swap_usage_warning_percent"], "Swap usage warning", "%")}
          {num(["alerts", "daemon_memory_warning_mb"], "Daemon memory warning", "MB")}
          {num(["alerts", "load_avg_multiplier_warning"], "Load average warning", "× cores", undefined, "w-32")}
          {num(["alerts", "trigger_debounce_checks"], "Open an alert after", "checks", "Consecutive failing checks before an alert opens.", "w-32")}
          {num(["alerts", "resolve_debounce_checks"], "Resolve an alert after", "checks", "Consecutive healthy checks before it resolves.", "w-32")}
        </div>
      </Card>

      <Card
        title={
          <>
            <Icon name="bell" size={15} />
            Notifications
          </>
        }
      >
        <div className="flex flex-col gap-3">
          <Row
            label="Browser notifications"
            badge={notifyBadge(notify)}
            hint={notifyHint(notify)}
          >
            <Switch
              disabled={notify.support !== "ok" || notify.permission === "denied"}
              checked={notify.preferred && notify.permission === "granted"}
              onChange={(on) => void setBrowserNotifications(on)}
            />
          </Row>

          {text(
            ["alerts", "webhook_url"],
            "Webhook URL",
            "https://example.com/hooks/routeviewnet",
            "Posts alert details to this address when an alert opens or resolves — the only channel that reaches you with the dashboard closed. Leave empty to turn off.",
          )}
          {num(["alerts", "webhook_timeout_seconds"], "Give up a delivery after", "s")}

          <div className="flex flex-wrap items-center gap-3 border-t pt-3" style={{ borderColor: "var(--border)" }}>
            <button
              onClick={() => {
                setTestNotice("");
                test.mutate();
              }}
              disabled={test.isPending}
              className="inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-sm transition-colors hover:border-[var(--series-1)] disabled:opacity-50"
              style={{ borderColor: "var(--border)" }}
            >
              <Icon name="send" size={14} />
              {test.isPending ? "Sending…" : "Send test alert"}
            </button>
            <span className="text-xs" style={{ color: testNotice ? "var(--ink-secondary)" : "var(--ink-muted)" }}>
              {testNotice || "Uses the saved webhook URL — save first if you just changed it."}
            </span>
          </div>
        </div>
      </Card>

      <Card
        title={
          <>
            <Icon name="shield" size={15} />
            Privacy &amp; access
          </>
        }
      >
        <div className="flex flex-col gap-3">
          {toggle(["privacy", "telemetry"], "Telemetry", "Off by default; nothing leaves this machine.")}
          {toggle(
            ["privacy", "store_process_command"],
            "Store process command lines",
            "May capture secrets and tokens passed as arguments. Off by default.",
          )}
          {toggle(["devices", "resolve_hostnames"], "Reverse-DNS device hostnames")}
          <Row
            label="LAN access"
            badge={<Badge icon="restart">needs restart</Badge>}
            hint={
              <span style={{ color: lanEnabled ? "var(--status-warning)" : undefined }}>
                Serves the dashboard to your whole network instead of this machine only. There is no
                login, so anyone who can reach this machine can read and change everything here.
              </span>
            }
          >
            <Switch checked={lanEnabled} onChange={(on) => set(["server", "host"], on ? "0.0.0.0" : "127.0.0.1")} />
          </Row>
        </div>
      </Card>

      {/* The action bar follows the page rather than sitting at the bottom of a
          long scroll, and only appears once there is something to save. */}
      <div className="sticky bottom-4 z-10">
        {changes > 0 ? (
          <div
            className="card flex flex-wrap items-center gap-3 p-3 shadow-lg"
            style={{ borderColor: "var(--series-1)" }}
          >
            <span className="mr-auto text-sm">
              {changes} unsaved {changes === 1 ? "change" : "changes"}
            </span>
            <button
              onClick={() => {
                setNotice("");
                setDraft(structuredClone(data!));
              }}
              disabled={save.isPending}
              className="inline-flex items-center gap-2 rounded-lg border px-3 py-1.5 text-sm disabled:opacity-50"
              style={{ borderColor: "var(--border)", color: "var(--ink-secondary)" }}
            >
              <Icon name="undo" size={14} />
              Revert
            </button>
            <button
              onClick={() => save.mutate(draft)}
              disabled={save.isPending}
              className="inline-flex items-center gap-2 rounded-lg px-4 py-1.5 text-sm font-medium disabled:opacity-60"
              style={{ background: "var(--series-1)", color: "#fff" }}
            >
              <Icon name="check" size={14} />
              {save.isPending ? "Saving…" : "Save changes"}
            </button>
          </div>
        ) : (
          notice && (
            <div
              className="card flex items-center gap-2 p-3 text-sm"
              style={{ color: "var(--ink-secondary)" }}
            >
              <Icon name={notice.startsWith("Save failed") ? "warning" : "check"} size={14} />
              {notice}
            </div>
          )
        )}
      </div>
    </div>
  );
}

// countChanges reports how many leaf settings differ, so the action bar can
// say how much is pending rather than just that something is.
function countChanges(a: any, b: any): number {
  if (a === b) return 0;
  if (Array.isArray(a) || Array.isArray(b) || typeof a !== "object" || a === null || b === null) {
    return JSON.stringify(a) === JSON.stringify(b) ? 0 : 1;
  }
  return Object.keys({ ...a, ...b }).reduce((n, k) => n + countChanges(a[k], b[k]), 0);
}

type NotifyState = { support: string; permission: string; preferred: boolean };

// The browser-notification toggle can be unavailable for reasons the user
// cannot see, so the state is labelled rather than left as a dead switch.
function notifyBadge(notify: NotifyState) {
  if (notify.support === "insecure-context") return <Badge icon="block">unavailable here</Badge>;
  if (notify.support === "unsupported") return <Badge icon="block">unsupported</Badge>;
  if (notify.permission === "denied") return <Badge icon="block">blocked</Badge>;
  return null;
}

// notifyHint explains what the toggle will and will not do. The
// insecure-context case is the one worth spelling out: the dashboard is
// reachable over the LAN on plain HTTP, where browsers do not expose the
// Notifications API at all, and a toggle that silently does nothing is worse
// than one that says why it cannot.
function notifyHint(notify: NotifyState): string {
  if (notify.support === "insecure-context") {
    return "Browsers only allow notifications on HTTPS or localhost, and this page is on a plain-HTTP LAN address. Open the dashboard on the machine running RouteViewNet, or use a webhook.";
  }
  if (notify.support === "unsupported") {
    return "This browser does not support notifications.";
  }
  if (notify.permission === "denied") {
    return "Blocked for this site in your browser settings. Re-allow it there to use this.";
  }
  if (notify.preferred && notify.permission === "granted") {
    return "Alerts appear as desktop notifications while a dashboard tab is open.";
  }
  return "Shows a desktop notification when an alert opens or resolves. Only works while a dashboard tab is open.";
}
