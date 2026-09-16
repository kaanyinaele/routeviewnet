import { ReactNode, useState } from "react";

// A "?" badge that reveals an explanation on hover, keyboard focus, or tap.
//
// Hover and focus-visible alone left touch users with no way to open a tip at
// all — the badge was a <span>, so tapping it did nothing. It is a real button
// now: pointer and keyboard keep the hover/focus behaviour through CSS, and a
// click toggles it open for everyone else. The tip text doubles as the
// accessible label.
export function InfoTip({ text }: { text: string }) {
  const [open, setOpen] = useState(false);
  return (
    <span className="group relative inline-flex">
      <button
        type="button"
        aria-label={text}
        aria-expanded={open}
        onClick={() => setOpen((o) => !o)}
        onBlur={() => setOpen(false)}
        className="flex h-4 w-4 cursor-help items-center justify-center rounded-full border text-[10px] leading-none"
        style={{ color: "var(--ink-muted)", borderColor: "var(--border)" }}
      >
        <span aria-hidden>?</span>
      </button>
      <span
        aria-hidden
        role="tooltip"
        className={`pointer-events-none absolute left-0 top-full z-20 mt-1.5 w-max max-w-64 rounded-md border px-2.5 py-1.5 text-xs font-normal shadow-lg group-hover:block group-focus-within:block ${
          open ? "block" : "hidden"
        }`}
        style={{
          background: "var(--surface-1)",
          borderColor: "var(--border)",
          color: "var(--ink-secondary)",
        }}
      >
        {text}
      </span>
    </span>
  );
}

export function Card({
  title,
  tip,
  children,
  className = "",
}: {
  title?: ReactNode;
  tip?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section className={`card p-4 ${className}`}>
      {title && (
        <h2
          className="mb-3 flex items-center gap-1.5 text-sm font-medium"
          style={{ color: "var(--ink-secondary)" }}
        >
          {title}
          {tip && <InfoTip text={tip} />}
        </h2>
      )}
      {children}
    </section>
  );
}

export function StatTile({
  label,
  value,
  sub,
  tip,
}: {
  label: string;
  value: ReactNode;
  sub?: ReactNode;
  tip?: string;
}) {
  return (
    <div className="card p-4">
      <div className="flex items-center gap-1.5 text-xs" style={{ color: "var(--ink-muted)" }}>
        <span>{label}</span>
        {tip && <InfoTip text={tip} />}
      </div>
      <div className="mt-1 text-2xl font-semibold">{value}</div>
      {sub && (
        <div className="mt-1 text-xs" style={{ color: "var(--ink-secondary)" }}>
          {sub}
        </div>
      )}
    </div>
  );
}

const statusColor: Record<string, string> = {
  ok: "var(--status-good)",
  healthy: "var(--status-good)",
  good: "var(--status-good)",
  info: "var(--series-1)",
  degraded: "var(--status-warning)",
  warning: "var(--status-warning)",
  failing: "var(--status-critical)",
  critical: "var(--status-critical)",
  unknown: "var(--ink-muted)",
  open: "var(--status-warning)",
  resolved: "var(--status-good)",
};

const statusIcon: Record<string, string> = {
  ok: "✓",
  healthy: "✓",
  good: "✓",
  resolved: "✓",
  info: "ℹ",
  degraded: "△",
  warning: "△",
  open: "△",
  failing: "✕",
  critical: "✕",
  unknown: "?",
};

// Three vocabularies reach the user at once — alert severity
// (info/warning/critical), health status (healthy/degraded/critical) and
// per-area status (ok/degraded/failing/unknown) — and nothing maps between
// them, so the same screen can show "degraded", "warning" and "failing" for
// overlapping ideas. The wire values are fixed: they are in SQLite, in the
// webhook contract and in docs/api.md. So they are translated here, at the
// one place every status already passes through, and the raw value stays
// available in the title attribute.
const statusLabel: Record<string, string> = {
  ok: "Working",
  healthy: "Working",
  good: "Working",
  degraded: "Slower than usual",
  failing: "Not working",
  critical: "Not working",
  unknown: "Not measured yet",
  info: "For information",
  warning: "Worth a look",
  open: "Happening now",
  resolved: "Fixed",
};

// Status is never color-alone: icon + label always ship together.
export function StatusPill({ status, label }: { status: string; label?: string }) {
  const color = statusColor[status] ?? "var(--ink-muted)";
  return (
    <span
      title={status}
      className="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium"
      style={{ color, borderColor: "var(--border)" }}
    >
      <span aria-hidden>{statusIcon[status] ?? "?"}</span>
      {label ?? statusLabel[status] ?? status}
    </span>
  );
}

export function Th({ children, className = "" }: { children?: ReactNode; className?: string }) {
  return (
    <th
      className={`px-3 py-2 text-left text-xs font-medium ${className}`}
      style={{ color: "var(--ink-muted)" }}
    >
      {children}
    </th>
  );
}

export function Td({ children, className = "" }: { children?: ReactNode; className?: string }) {
  return <td className={`px-3 py-2 text-sm ${className}`}>{children}</td>;
}

export function Empty({ text }: { text: string }) {
  return (
    <div className="py-8 text-center text-sm" style={{ color: "var(--ink-muted)" }}>
      {text}
    </div>
  );
}

// PageState distinguishes the three cases every page has and most pages used
// to collapse into one.
//
// Seven of the eight pages read only `data` from useQuery and never looked at
// `error`, so a failing request rendered the same "no data yet" line as a
// healthy-but-idle collector — teaching the user that the feature is broken
// when the server is simply down. Troubleshoot was worse: it returned
// "Loading…" and stayed there forever.
//
// `empty` is deliberately the caller's sentence, because only the page knows
// whether nothing-yet means "give the collector a minute" or "you have no
// devices". `error` is shared, because the recovery is always the same.
export function PageState({
  isLoading,
  error,
  isEmpty,
  empty,
  children,
}: {
  isLoading?: boolean;
  error?: unknown;
  isEmpty?: boolean;
  empty?: string;
  children?: ReactNode;
}) {
  if (error) {
    return (
      <div className="py-8 text-center text-sm" style={{ color: "var(--ink-secondary)" }}>
        <div style={{ color: "var(--status-warning)" }}>{friendlyError(error)}</div>
        <div className="mt-1 text-xs" style={{ color: "var(--ink-muted)" }}>
          This page will fill in by itself once it can read the monitor again.
        </div>
      </div>
    );
  }
  if (isLoading) return <Empty text="Loading…" />;
  if (isEmpty) return <Empty text={empty ?? "Nothing to show yet."} />;
  return <>{children}</>;
}

// friendlyError turns a thrown fetch/HTTP error into something a person can
// act on. The common case is not an HTTP status at all — it is the daemon
// being stopped, which surfaces as a bare "TypeError: Failed to fetch".
export function friendlyError(e: unknown): string {
  const raw = e instanceof Error ? e.message : String(e);
  if (/failed to fetch|networkerror|load failed/i.test(raw)) {
    return "RouteViewNet's background monitor isn't answering. It may have stopped — try: sudo systemctl restart routeviewnetd";
  }
  if (/^HTTP 5/.test(raw)) {
    return `The monitor ran into a problem answering this page (${raw}).`;
  }
  return raw;
}

export function RangePicker({ value, onChange }: { value: string; onChange: (r: any) => void }) {
  return (
    <div className="flex gap-1">
      {["15m", "1h", "6h", "24h", "7d"].map((r) => (
        <button
          key={r}
          onClick={() => onChange(r)}
          className={`rounded-md px-2.5 py-1 text-xs ${
            value === r ? "bg-[var(--surface-1)] border border-[var(--border)]" : ""
          }`}
          style={{ color: value === r ? "var(--ink-primary)" : "var(--ink-muted)" }}
        >
          {r}
        </button>
      ))}
    </div>
  );
}
