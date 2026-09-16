import { useQuery } from "@tanstack/react-query";
import { api } from "../lib/api";
import { Card, Empty, PageState, StatusPill } from "../components/ui";

export default function TroubleshootPage() {
  const { data: d, error, isLoading } = useQuery({
    queryKey: ["troubleshoot"],
    queryFn: api.troubleshoot,
  });

  // Previously `if (!d) return "Loading…"`, which never cleared on failure:
  // the page sat on that word forever whenever the request errored.
  if (!d || error || isLoading) {
    return (
      <div className="flex flex-col gap-4">
        <h1 className="text-xl font-semibold">Troubleshooting</h1>
        <Card>
          <PageState error={error} isLoading={isLoading} isEmpty={!d} empty="No diagnosis yet." />
        </Card>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-xl font-semibold">Troubleshooting</h1>

      <Card>
        <div className="flex items-center gap-3">
          <StatusPill status={d.status} />
          <span className="text-lg font-medium">{d.summary}</span>
        </div>
        <p className="mt-3 text-sm" style={{ color: "var(--ink-secondary)" }}>
          {d.likely_cause}
        </p>
      </Card>

      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <Card title="Evidence">
          {d.evidence?.length ? (
            <ul className="flex flex-col gap-2 text-sm">
              {d.evidence.map((e, i) => (
                <li key={i} className="flex gap-2">
                  <span aria-hidden style={{ color: "var(--ink-muted)" }}>
                    •
                  </span>
                  {e}
                </li>
              ))}
            </ul>
          ) : (
            <Empty text="No supporting evidence." />
          )}
        </Card>

        <Card title="Suggested actions">
          {d.suggested_actions?.length ? (
            <ol className="flex list-inside list-decimal flex-col gap-2 text-sm">
              {d.suggested_actions.map((a, i) => (
                <li key={i}>{a}</li>
              ))}
            </ol>
          ) : (
            <Empty text="Nothing to do. All healthy." />
          )}
        </Card>
      </div>

      <p className="text-xs" style={{ color: "var(--ink-muted)" }}>
        Diagnosis is rule-based, computed from the currently open alerts. It updates automatically as
        conditions change.
      </p>
    </div>
  );
}
