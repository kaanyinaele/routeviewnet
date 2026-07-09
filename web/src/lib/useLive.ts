import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

// Maps WebSocket event types (§10.3) to the query keys they invalidate.
const invalidations: Record<string, string[][]> = {
  "metrics.interface.updated": [["overview"], ["bandwidth"], ["interfaces"]],
  "device.discovered": [["devices"], ["overview"]],
  "device.updated": [["devices"]],
  "alert.created": [["alerts"], ["overview"], ["troubleshoot"]],
  "alert.resolved": [["alerts"], ["overview"], ["troubleshoot"]],
  "health.updated": [["overview"], ["healthHistory"]],
  "dns.check.updated": [["dnsChecks"]],
  "latency.check.updated": [["latencyChecks"]],
  "system.memory.updated": [["memory"], ["memoryHistory"], ["overview"]],
  "system.process_memory.updated": [["processes"]],
  "system.daemon_memory.updated": [["daemonMemory"]],
  "system.load.updated": [["load"], ["overview"]],
};

// useLive keeps a WebSocket to /api/v1/live and invalidates the affected
// queries on each event, with throttling so 5s collector cycles don't
// hammer the render loop. Reconnects with backoff.
export function useLive(): boolean {
  const qc = useQueryClient();
  const [connected, setConnected] = useState(false);
  const lastInvalidate = useRef<Record<string, number>>({});

  useEffect(() => {
    let ws: WebSocket | null = null;
    let closed = false;
    let retryMs = 1000;

    const connect = () => {
      const proto = location.protocol === "https:" ? "wss" : "ws";
      ws = new WebSocket(`${proto}://${location.host}/api/v1/live`);
      ws.onopen = () => {
        setConnected(true);
        retryMs = 1000;
      };
      ws.onmessage = (msg) => {
        try {
          const ev = JSON.parse(msg.data);
          const keys = invalidations[ev.type];
          if (!keys) return;
          const now = Date.now();
          for (const key of keys) {
            const k = key.join(".");
            if (now - (lastInvalidate.current[k] ?? 0) < 2000) continue;
            lastInvalidate.current[k] = now;
            qc.invalidateQueries({ queryKey: key });
          }
        } catch {
          /* ignore malformed frames */
        }
      };
      ws.onclose = () => {
        setConnected(false);
        if (!closed) {
          setTimeout(connect, retryMs);
          retryMs = Math.min(retryMs * 2, 15000);
        }
      };
    };
    connect();
    return () => {
      closed = true;
      ws?.close();
    };
  }, [qc]);

  return connected;
}
