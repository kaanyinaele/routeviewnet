import { ReactNode } from "react";

// A "?" badge that reveals an explanation on hover or keyboard focus.
// The tip text doubles as the accessible label for screen readers.
export function InfoTip({ text }: { text: string }) {
  return (
    <span className="group relative inline-flex" tabIndex={0} aria-label={text}>
      <span
        aria-hidden
        className="flex h-4 w-4 cursor-help items-center justify-center rounded-full border text-[10px] leading-none"
        style={{ color: "var(--ink-muted)", borderColor: "var(--border)" }}
      >
        ?
      </span>
      <span
        aria-hidden
        role="tooltip"
        className="pointer-events-none absolute left-0 top-full z-20 mt-1.5 hidden w-max max-w-64 rounded-md border px-2.5 py-1.5 text-xs font-normal shadow-lg group-hover:block group-focus-visible:block"
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

// Status is never color-alone: icon + label always ship together.
export function StatusPill({ status, label }: { status: string; label?: string }) {
  const color = statusColor[status] ?? "var(--ink-muted)";
  return (
    <span
      className="inline-flex items-center gap-1.5 rounded-full border px-2 py-0.5 text-xs font-medium"
      style={{ color, borderColor: "var(--border)" }}
    >
      <span aria-hidden>{statusIcon[status] ?? "?"}</span>
      {label ?? status}
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
