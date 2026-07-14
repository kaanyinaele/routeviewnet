import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, Device, timeAgo } from "../lib/api";
import { Card, Empty, StatusPill, Td, Th } from "../components/ui";

export default function DevicesPage() {
  const qc = useQueryClient();
  const { data } = useQuery({ queryKey: ["devices"], queryFn: api.devices });
  const { data: overview } = useQuery({ queryKey: ["overview"], queryFn: api.overview });
  const gatewayIP = overview?.gateway_ip;
  const patch = useMutation({
    mutationFn: ({ id, body }: { id: number; body: { nickname?: string; trusted?: boolean } }) =>
      api.patchDevice(id, body),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["devices"] }),
  });

  return (
    <div className="flex flex-col gap-4">
      <h1 className="text-xl font-semibold">Devices</h1>
      <p className="text-sm" style={{ color: "var(--ink-muted)" }}>
        {data?.note ?? "Devices visible from this Linux machine."}
      </p>
      <Card>
        {data?.items?.length ? (
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead>
                <tr className="border-b" style={{ borderColor: "var(--border)" }}>
                  <Th>IP</Th>
                  <Th>MAC</Th>
                  <Th>Hostname</Th>
                  <Th>Vendor</Th>
                  <Th>Nickname</Th>
                  <Th>Trust</Th>
                  <Th>State</Th>
                  <Th>First seen</Th>
                  <Th>Last seen</Th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((d) => (
                  <DeviceRow
                    key={d.id}
                    device={d}
                    isGateway={!!gatewayIP && d.ip_address === gatewayIP}
                    onPatch={(body) => patch.mutate({ id: d.id, body })}
                  />
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty text="No devices discovered yet. Discovery reads this machine's IPv4 neighbor table (ip neigh)." />
        )}
      </Card>
    </div>
  );
}

function DeviceRow({
  device: d,
  isGateway,
  onPatch,
}: {
  device: Device;
  isGateway: boolean;
  onPatch: (b: { nickname?: string; trusted?: boolean }) => void;
}) {
  const [editing, setEditing] = useState(false);
  const [nickname, setNickname] = useState(d.nickname);

  return (
    <tr className="border-b" style={{ borderColor: "var(--border)" }}>
      <Td className="tabular">
        <span className="inline-flex items-center gap-2">
          {d.ip_address}
          {isGateway && <StatusPill status="info" label="your router" />}
        </span>
      </Td>
      <Td className="tabular">{d.mac_address || "-"}</Td>
      <Td>{d.hostname || "-"}</Td>
      <Td>{d.vendor || "-"}</Td>
      <Td>
        {editing ? (
          <form
            className="flex gap-1"
            onSubmit={(e) => {
              e.preventDefault();
              onPatch({ nickname });
              setEditing(false);
            }}
          >
            <input
              autoFocus
              value={nickname}
              maxLength={100}
              onChange={(e) => setNickname(e.target.value)}
              className="w-32 rounded border bg-transparent px-1 py-0.5 text-sm"
              style={{ borderColor: "var(--border)" }}
            />
            <button type="submit" className="text-xs" style={{ color: "var(--series-1)" }}>
              save
            </button>
          </form>
        ) : (
          <button onClick={() => setEditing(true)} className="text-left">
            {d.nickname || (
              <span className="text-xs" style={{ color: "var(--ink-muted)" }}>
                + add
              </span>
            )}
          </button>
        )}
      </Td>
      <Td>
        <button onClick={() => onPatch({ trusted: !d.trusted })}>
          {d.trusted ? (
            <StatusPill status="ok" label="trusted" />
          ) : (
            <StatusPill status="warning" label="new / untrusted" />
          )}
        </button>
      </Td>
      <Td>{d.state || "-"}</Td>
      <Td>{timeAgo(d.first_seen_at)}</Td>
      <Td>{timeAgo(d.last_seen_at)}</Td>
    </tr>
  );
}
