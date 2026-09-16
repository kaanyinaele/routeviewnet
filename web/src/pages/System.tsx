import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, formatBytes, Range } from "../lib/api";
import { Card, PageState, RangePicker, StatTile, Td, Th } from "../components/ui";
import { TimeSeriesChart } from "../components/charts";

export default function SystemPage() {
  const [range, setRange] = useState<Range>("1h");
  const { data: mem } = useQuery({ queryKey: ["memory"], queryFn: api.memory });
  const { data: load } = useQuery({ queryKey: ["load"], queryFn: api.load });
  const { data: daemon } = useQuery({ queryKey: ["daemonMemory"], queryFn: api.daemonMemory });
  const { data: procs, error: procsError, isLoading: procsLoading } = useQuery({
    queryKey: ["processes"],
    queryFn: api.processes,
  });
  const { data: hist, error: histError, isLoading: histLoading } = useQuery({
    queryKey: ["memoryHistory", range],
    queryFn: () => api.memoryHistory(range),
  });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">System</h1>
        <RangePicker value={range} onChange={setRange} />
      </div>

      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        <StatTile
          label="Memory used"
          value={mem ? `${mem.memory_usage_percent.toFixed(0)}%` : "-"}
          sub={mem ? `${formatBytes(mem.mem_used_bytes)} of ${formatBytes(mem.mem_total_bytes)}` : undefined}
          tip="How much of your computer's working memory (RAM) is truly busy. Linux borrows spare memory to speed things up and returns it when programs need it, so that part doesn't count."
        />
        <StatTile
          label="Swap used"
          value={mem ? `${mem.swap_usage_percent.toFixed(0)}%` : "-"}
          sub={mem ? `${formatBytes(mem.swap_used_bytes)} of ${formatBytes(mem.swap_total_bytes)}` : undefined}
          tip="Overflow space on the disk that gets used when memory runs low. The disk is much slower than memory, so heavy use here is the classic reason a computer feels sluggish."
        />
        <StatTile
          label="Load average"
          value={load ? load.load_avg_1m.toFixed(2) : "-"}
          sub={
            load
              ? `5m ${load.load_avg_5m.toFixed(2)} · 15m ${load.load_avg_15m.toFixed(2)} · ${load.cpu_core_count} cores`
              : undefined
          }
          tip="How busy the processor has been over the last 1, 5, and 15 minutes. Trouble starts when the 1-minute number stays above the number of processor cores."
        />
        <StatTile
          label="Daemon (RouteViewNet)"
          value={daemon ? formatBytes(daemon.rss_bytes) : "-"}
          sub={daemon ? `heap ${formatBytes(daemon.heap_alloc_bytes)} · ${daemon.goroutines} goroutines` : undefined}
          tip="RouteViewNet's own memory use. It should stay small, typically less than a single browser tab."
        />
      </div>

      <Card
        title="Memory & swap usage"
        tip="Memory and swap use over the selected time window. A line that climbs and never comes down can mean a program is slowly leaking memory."
      >
        <PageState
          error={histError}
          isLoading={histLoading}
          isEmpty={!hist?.points?.length}
          empty="No memory samples yet. Give the collector a minute."
        >
          <TimeSeriesChart
            data={hist?.points ?? []}
            yDomain={[0, 100]}
            format={(v) => `${v.toFixed(0)}%`}
            series={[
              { key: "memory_usage_percent", name: "Memory", color: "var(--series-1)" },
              { key: "swap_usage_percent", name: "Swap", color: "var(--series-2)" },
            ]}
          />
        </PageState>
      </Card>

      <Card
        title="Top memory consumers"
        tip="The programs using the most memory right now, biggest first. If the computer feels slow, the culprit is usually near the top of this list."
      >
        <PageState
          error={procsError}
          isLoading={procsLoading}
          isEmpty={!procs?.items?.length}
          empty="No process scan yet (runs every 30s by default)."
        >
          <table className="w-full">
            <thead>
              <tr className="border-b" style={{ borderColor: "var(--border)" }}>
                <Th>PID</Th>
                <Th>Process</Th>
                <Th>RSS</Th>
                <Th>% of RAM</Th>
              </tr>
            </thead>
            <tbody>
              {procs?.items?.map((p) => (
                <tr key={p.pid} className="border-b" style={{ borderColor: "var(--border)" }}>
                  <Td className="tabular">{p.pid}</Td>
                  <Td>{p.process_name}</Td>
                  <Td className="tabular">{formatBytes(p.rss_bytes)}</Td>
                  <Td className="tabular">{p.memory_percent.toFixed(1)}%</Td>
                </tr>
              ))}
            </tbody>
          </table>
        </PageState>
      </Card>
    </div>
  );
}
