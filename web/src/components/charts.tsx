import {
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
  CartesianGrid,
} from "recharts";
import { formatRate } from "../lib/api";

const tickStyle = { fill: "var(--ink-muted)", fontSize: 11 };

function timeTick(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function ChartTooltip({ active, payload, label, format }: any) {
  if (!active || !payload?.length) return null;
  return (
    <div className="card px-3 py-2 text-xs" style={{ background: "var(--surface-1)" }}>
      <div style={{ color: "var(--ink-muted)" }}>{label ? new Date(label).toLocaleString() : ""}</div>
      {payload.map((p: any) => (
        <div key={p.dataKey} className="mt-1 flex items-center gap-2">
          <span aria-hidden className="inline-block h-2 w-2 rounded-full" style={{ background: p.stroke }} />
          <span style={{ color: "var(--ink-secondary)" }}>{p.name}</span>
          <span className="tabular ml-auto font-medium" style={{ color: "var(--ink-primary)" }}>
            {format ? format(p.value) : p.value}
          </span>
        </div>
      ))}
    </div>
  );
}

export interface SeriesSpec {
  key: string;
  name: string;
  color: string;
}

// TimeSeriesChart: 2px lines, recessive hairline grid, crosshair tooltip
// (dataviz mark specs). Legend rendered inline above the plot.
export function TimeSeriesChart({
  data,
  series,
  format,
  height = 220,
  yDomain,
}: {
  data: any[];
  series: SeriesSpec[];
  format?: (v: number) => string;
  height?: number;
  yDomain?: [number | string, number | string];
}) {
  return (
    <div>
      {series.length > 1 && (
        <div className="mb-2 flex gap-4 text-xs" style={{ color: "var(--ink-secondary)" }}>
          {series.map((s) => (
            <span key={s.key} className="inline-flex items-center gap-1.5">
              <span aria-hidden className="inline-block h-2 w-2 rounded-full" style={{ background: s.color }} />
              {s.name}
            </span>
          ))}
        </div>
      )}
      <ResponsiveContainer width="100%" height={height}>
        <LineChart data={data} margin={{ top: 4, right: 8, bottom: 0, left: 0 }}>
          <CartesianGrid stroke="var(--grid)" strokeWidth={1} vertical={false} />
          <XAxis
            dataKey="time"
            tickFormatter={timeTick}
            tick={tickStyle}
            stroke="var(--baseline)"
            tickLine={false}
            minTickGap={48}
          />
          <YAxis
            tick={tickStyle}
            stroke="transparent"
            tickLine={false}
            width={56}
            domain={yDomain}
            tickFormatter={(v: number) => (format ? format(v) : String(v))}
          />
          <Tooltip
            content={<ChartTooltip format={format} />}
            cursor={{ stroke: "var(--baseline)", strokeWidth: 1 }}
          />
          {series.map((s) => (
            <Line
              key={s.key}
              type="monotone"
              dataKey={s.key}
              name={s.name}
              stroke={s.color}
              strokeWidth={2}
              dot={false}
              activeDot={{ r: 4, stroke: "var(--surface-1)", strokeWidth: 2 }}
              isAnimationActive={false}
            />
          ))}
        </LineChart>
      </ResponsiveContainer>
    </div>
  );
}

export function BandwidthChart({ points, height = 220 }: { points: any[]; height?: number }) {
  return (
    <TimeSeriesChart
      data={points}
      height={height}
      format={formatRate}
      series={[
        { key: "rx_rate_bps", name: "Download", color: "var(--series-1)" },
        { key: "tx_rate_bps", name: "Upload", color: "var(--series-2)" },
      ]}
    />
  );
}

// Sparkline: single-series, chrome-free trend line. Defaults to the
// health score's 0-100 domain; pass a domain for other units.
export function Sparkline({
  points,
  dataKey,
  color,
  domain = [0, 100],
}: {
  points: any[];
  dataKey: string;
  color: string;
  domain?: [number | string, number | string];
}) {
  return (
    <ResponsiveContainer width="100%" height={48}>
      <LineChart data={points} margin={{ top: 4, right: 0, bottom: 0, left: 0 }}>
        <YAxis hide domain={domain} />
        <Line type="monotone" dataKey={dataKey} stroke={color} strokeWidth={2} dot={false} isAnimationActive={false} />
      </LineChart>
    </ResponsiveContainer>
  );
}
