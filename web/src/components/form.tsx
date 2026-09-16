import { ReactNode } from "react";
import { Icon, IconName } from "./icons";

// Form primitives for the settings document. Presentational only — the page
// owns the draft state and passes values down.

// Row is one setting: name, optional explanation, optional badge, control.
// `stacked` puts a full-width control under the label instead of beside it,
// which is what the free-text and list settings need.
export function Row({
  label,
  hint,
  badge,
  stacked = false,
  children,
}: {
  label: ReactNode;
  hint?: ReactNode;
  badge?: ReactNode;
  stacked?: boolean;
  children: ReactNode;
}) {
  return (
    <label
      className={
        stacked
          ? "flex flex-col gap-1.5 text-sm"
          : "flex items-center justify-between gap-4 text-sm"
      }
    >
      <span className={stacked ? "" : "min-w-0"}>
        <span className="flex flex-wrap items-center gap-2">
          {label}
          {badge}
        </span>
        {hint && (
          <span className="mt-0.5 block text-xs leading-relaxed" style={{ color: "var(--ink-muted)" }}>
            {hint}
          </span>
        )}
      </span>
      {children}
    </label>
  );
}

// Switch is a real checkbox with the box hidden, so labels, keyboard focus
// and form semantics keep working — only the rendering changes. The native
// control the browser draws is bright system blue, which fights the palette.
export function Switch({
  checked,
  onChange,
  disabled = false,
}: {
  checked: boolean;
  onChange: (on: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <span className={`relative inline-flex shrink-0 ${disabled ? "opacity-40" : ""}`}>
      <input
        type="checkbox"
        className="peer sr-only"
        checked={checked}
        disabled={disabled}
        onChange={(e) => onChange(e.target.checked)}
      />
      {/* Track and knob are styled from `checked` inline rather than with
          peer-checked: classes — an inline style beats a class, so the two
          cannot be mixed on the same property. */}
      <span
        aria-hidden
        className="h-5 w-9 rounded-full border transition-colors peer-focus-visible:ring-2 peer-focus-visible:ring-[var(--series-1)] peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-[var(--surface-1)]"
        style={{
          borderColor: checked ? "transparent" : "var(--border)",
          background: checked ? "var(--series-1)" : "var(--surface-0)",
        }}
      />
      <span
        aria-hidden
        className="pointer-events-none absolute left-0.5 top-0.5 h-4 w-4 rounded-full transition-transform"
        style={{
          background: checked ? "#fff" : "var(--ink-muted)",
          transform: checked ? "translateX(1rem)" : "none",
        }}
      />
    </span>
  );
}

// NumberField keeps the unit inside the control so labels stay plain language
// ("Network check interval", not "Network check interval (s)").
export function NumberField({
  value,
  onChange,
  unit,
  width = "w-28",
}: {
  value: number;
  onChange: (n: number) => void;
  unit?: string;
  width?: string;
}) {
  return (
    <span
      className={`inline-flex shrink-0 items-center gap-1 rounded-md border px-2 py-1 focus-within:border-[var(--series-1)] ${width}`}
      style={{ borderColor: "var(--border)", background: "var(--surface-0)" }}
    >
      <input
        type="number"
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
        className="tabular w-full min-w-0 bg-transparent text-right outline-none"
      />
      {unit && (
        <span className="shrink-0 text-xs" style={{ color: "var(--ink-muted)" }}>
          {unit}
        </span>
      )}
    </span>
  );
}

export function TextField({
  value,
  onChange,
  placeholder,
  mono = false,
}: {
  value: string;
  onChange: (s: string) => void;
  placeholder?: string;
  mono?: boolean;
}) {
  return (
    <input
      value={value}
      placeholder={placeholder}
      onChange={(e) => onChange(e.target.value)}
      className={`w-full rounded-md border px-2.5 py-1.5 outline-none focus:border-[var(--series-1)] ${mono ? "tabular" : ""}`}
      style={{ borderColor: "var(--border)", background: "var(--surface-0)" }}
    />
  );
}

export function Badge({
  icon,
  children,
  tone = "muted",
}: {
  icon?: IconName;
  children: ReactNode;
  tone?: "muted" | "warning";
}) {
  const color = tone === "warning" ? "var(--status-warning)" : "var(--ink-muted)";
  return (
    <span
      className="inline-flex items-center gap-1 rounded-full border px-1.5 py-0.5 text-[11px] font-medium"
      style={{ color, borderColor: "var(--border)" }}
    >
      {icon && <Icon name={icon} size={11} />}
      {children}
    </span>
  );
}

// Subhead separates groups inside one card — the collection card mixes "how
// often to sample" with "what to sample at all", which are different decisions.
export function Subhead({ icon, children }: { icon: IconName; children: ReactNode }) {
  return (
    <div
      className="flex items-center gap-1.5 border-t pt-3 text-xs font-medium first:border-t-0 first:pt-0"
      style={{ color: "var(--ink-muted)", borderColor: "var(--border)" }}
    >
      <Icon name={icon} size={13} />
      {children}
    </div>
  );
}
