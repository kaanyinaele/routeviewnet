// Typed client for the RouteViewNet /api/v1 endpoints.

export interface HealthScore {
  score: number;
  status: "healthy" | "degraded" | "critical";
  computed_at: string;
}

export interface StatusSummary {
  status: "ok" | "degraded" | "failing" | "unknown";
  latency_ms: number;
}

export interface InterfaceMetric {
  interface_name: string;
  rx_bytes: number;
  tx_bytes: number;
  rx_rate_bps: number;
  tx_rate_bps: number;
  rx_dropped: number;
  tx_dropped: number;
  rx_errors: number;
  tx_errors: number;
  has_rates: boolean;
  collected_at: string;
}

export interface MemoryMetrics {
  mem_total_bytes: number;
  mem_available_bytes: number;
  mem_used_bytes: number;
  swap_total_bytes: number;
  swap_used_bytes: number;
  memory_usage_percent: number;
  swap_usage_percent: number;
  collected_at: string;
}

export interface LoadMetrics {
  load_avg_1m: number;
  load_avg_5m: number;
  load_avg_15m: number;
  cpu_core_count: number;
}

export interface DaemonMemory {
  rss_bytes: number;
  heap_alloc_bytes: number;
  goroutines: number;
}

export interface EventRow {
  id: number;
  type: string;
  severity: string;
  payload: string;
  created_at: string;
}

export interface Overview {
  health: HealthScore;
  primary_interface: string;
  gateway_ip: string;
  bandwidth: InterfaceMetric | null;
  gateway: StatusSummary;
  internet: StatusSummary;
  dns: StatusSummary;
  memory: MemoryMetrics | null;
  daemon_memory: DaemonMemory | null;
  load: LoadMetrics | null;
  device_count: number;
  open_alert_count: number;
  recent_events: EventRow[];
}

export interface NetworkInterface {
  id: number;
  name: string;
  mac_address: string;
  state: string;
  speed_mbps: number;
  is_primary: boolean;
}

export interface BandwidthPoint {
  time: string;
  rx_rate_bps: number;
  tx_rate_bps: number;
  rx_dropped: number;
  tx_dropped: number;
  rx_errors: number;
  tx_errors: number;
}

export interface Device {
  id: number;
  ip_address: string;
  mac_address: string;
  hostname: string;
  vendor: string;
  nickname: string;
  trusted: boolean;
  interface_name: string;
  state: string;
  first_seen_at: string | null;
  last_seen_at: string | null;
}

export interface Alert {
  id: number;
  rule_key: string;
  severity: "info" | "warning" | "critical";
  title: string;
  message: string;
  source: string;
  status: "open" | "resolved";
  occurrence_count: number;
  created_at: string;
  resolved_at: string | null;
}

export interface LatencyCheck {
  target: string;
  target_type: string;
  method: string;
  latency_ms: number;
  packet_loss: number;
  success: boolean;
  collected_at: string;
}

export interface DNSCheck {
  domain: string;
  record_type: string;
  resolver: string;
  latency_ms: number;
  success: boolean;
  collected_at: string;
}

export interface ProcessMemory {
  pid: number;
  process_name: string;
  command?: string;
  rss_bytes: number;
  memory_percent: number;
}

export interface MemoryHistoryPoint {
  time: string;
  memory_usage_percent: number;
  swap_usage_percent: number;
  mem_used_bytes: number;
}

export interface Diagnosis {
  status: string;
  summary: string;
  likely_cause: string;
  evidence: string[];
  suggested_actions: string[];
}

export interface AppTrafficTotal {
  app: string;
  rx_bytes: number;
  tx_bytes: number;
  total_bytes: number;
}

export type Range = "15m" | "1h" | "6h" | "24h" | "7d";

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`/api/v1${path}`);
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new Error(body?.error?.message ?? `HTTP ${res.status}`);
  }
  return res.json();
}

export const api = {
  overview: () => get<Overview>("/overview"),
  interfaces: () => get<{ items: NetworkInterface[] }>("/interfaces"),
  bandwidth: (range: Range, iface?: string) =>
    get<{ interface: string; points: BandwidthPoint[] | null }>(
      `/metrics/bandwidth?range=${range}${iface ? `&interface=${encodeURIComponent(iface)}` : ""}`,
    ),
  appTraffic: (range: Range) =>
    get<{ items: AppTrafficTotal[] | null; note: string }>(`/traffic/apps?range=${range}`),
  devices: () => get<{ items: Device[] | null; note: string }>("/devices?limit=200"),
  patchDevice: async (id: number, body: { nickname?: string; trusted?: boolean }) => {
    const res = await fetch(`/api/v1/devices/${id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    return res.json() as Promise<Device>;
  },
  alerts: (status?: string) =>
    get<{ items: Alert[] | null }>(`/alerts?limit=100${status ? `&status=${status}` : ""}`),
  latencyChecks: (range: Range) => get<{ items: LatencyCheck[] | null }>(`/checks/latency?range=${range}`),
  dnsChecks: (range: Range) => get<{ items: DNSCheck[] | null }>(`/checks/dns?range=${range}`),
  memory: () => get<MemoryMetrics | null>("/system/memory"),
  memoryHistory: (range: Range) =>
    get<{ points: MemoryHistoryPoint[] | null }>(`/system/memory/history?range=${range}`),
  load: () => get<LoadMetrics | null>("/system/load"),
  processes: () => get<{ items: ProcessMemory[] | null }>("/system/processes/memory"),
  daemonMemory: () => get<DaemonMemory | null>("/system/daemon/memory"),
  healthHistory: (range: Range) =>
    get<{ points: HealthScore[] | null }>(`/health-score/history?range=${range}`),
  troubleshoot: () => get<Diagnosis>("/troubleshoot"),
  settings: () => get<Record<string, any>>("/settings"),
  saveSettings: async (settings: Record<string, any>) => {
    const res = await fetch("/api/v1/settings", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(settings),
    });
    const body = await res.json();
    if (!res.ok) throw new Error(body?.error?.message ?? `HTTP ${res.status}`);
    return body as { settings: Record<string, any>; restart_required: boolean };
  },
};

export function formatBytes(n: number, suffix = "B"): string {
  if (!isFinite(n) || n < 0) return "-";
  const units = ["", "K", "M", "G", "T"];
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i++;
  }
  return `${n >= 100 ? n.toFixed(0) : n.toFixed(1)} ${units[i]}${suffix}`;
}

export function formatRate(bps: number): string {
  return formatBytes(bps, "B/s");
}

export function timeAgo(iso: string | null): string {
  if (!iso) return "-";
  const s = (Date.now() - new Date(iso).getTime()) / 1000;
  if (s < 0) return "now";
  if (s < 60) return `${Math.floor(s)}s ago`;
  if (s < 3600) return `${Math.floor(s / 60)}m ago`;
  if (s < 86400) return `${Math.floor(s / 3600)}h ago`;
  return `${Math.floor(s / 86400)}d ago`;
}
