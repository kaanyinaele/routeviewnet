import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, timeAgo } from "../lib/api";
import { Card, Empty, StatusPill, Td, Th } from "../components/ui";

export default function AlertsPage() {
  const [status, setStatus] = useState<"open" | "resolved" | "">("open");
  const { data } = useQuery({
    queryKey: ["alerts", status],
    queryFn: () => api.alerts(status || undefined),
  });

  return (
    <div className="flex flex-col gap-4">
      <div className="flex items-center justify-between">
        <h1 className="text-xl font-semibold">Alerts</h1>
        <div className="flex gap-1">
          {(["open", "resolved", ""] as const).map((s) => (
            <button
              key={s || "all"}
              onClick={() => setStatus(s)}
              className={`rounded-md px-2.5 py-1 text-xs ${status === s ? "bg-[var(--surface-1)] border border-[var(--border)]" : ""}`}
              style={{ color: status === s ? "var(--ink-primary)" : "var(--ink-muted)" }}
            >
              {s || "all"}
            </button>
          ))}
        </div>
      </div>

      <Card>
        {data?.items?.length ? (
          <table className="w-full">
            <thead>
              <tr className="border-b" style={{ borderColor: "var(--border)" }}>
                <Th>Severity</Th>
                <Th>Alert</Th>
                <Th>Source</Th>
                <Th>Seen</Th>
                <Th>Opened</Th>
                <Th>Status</Th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((a) => (
                <tr key={a.id} className="border-b align-top" style={{ borderColor: "var(--border)" }}>
                  <Td>
                    <StatusPill status={a.severity} />
                  </Td>
                  <Td>
                    <div className="font-medium">{a.title}</div>
                    <div className="text-xs" style={{ color: "var(--ink-secondary)" }}>
                      {a.message}
                    </div>
                  </Td>
                  <Td className="tabular">{a.source || "-"}</Td>
                  <Td className="tabular">×{a.occurrence_count}</Td>
                  <Td className="tabular">{timeAgo(a.created_at)}</Td>
                  <Td>
                    {a.status === "resolved" ? (
                      <StatusPill status="resolved" label={`resolved ${timeAgo(a.resolved_at)}`} />
                    ) : (
                      <StatusPill status="open" />
                    )}
                  </Td>
                </tr>
              ))}
            </tbody>
          </table>
        ) : (
          <Empty text={status === "open" ? "No open alerts. Everything looks healthy." : "No alerts."} />
        )}
      </Card>
    </div>
  );
}
