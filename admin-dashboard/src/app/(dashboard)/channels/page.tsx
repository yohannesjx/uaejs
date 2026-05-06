"use client";

import { useQuery } from "@tanstack/react-query";
import { createColumnHelper } from "@tanstack/react-table";

import { PermissionGate } from "@/components/permission-gate";
import { DataTable } from "@/components/data-table";
import { PageHeader } from "@/components/dashboard-blocks";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/primitives";
import { api } from "@/lib/api-client";
import type { ExternalPlatform } from "@/types/api";

const channelColumn = createColumnHelper<ExternalPlatform>();

export default function ChannelsPage() {
  const channelsQuery = useQuery({
    queryKey: ["channels"],
    queryFn: api.listChannels,
  });

  return (
    <PermissionGate
      permission="channels.manage"
      fallback={<div className="text-sm text-[var(--muted-foreground)]">Missing channels.manage permission.</div>}
    >
      <div className="space-y-6">
        <PageHeader
          title="Omnichannel sync"
          description="Connected platforms and marketplace control center."
        />
        <Card>
          <CardHeader>
            <CardTitle>Platforms</CardTitle>
            <CardDescription>Backed by <code>/admin/channels</code>.</CardDescription>
          </CardHeader>
          <CardContent>
            <DataTable
              data={channelsQuery.data ?? []}
              columns={[
                channelColumn.accessor("name", { header: "Platform" }),
                channelColumn.accessor("type", { header: "Type" }),
                channelColumn.accessor("is_active", {
                  header: "Active",
                  cell: ({ getValue }) => (getValue() ? "Yes" : "No"),
                }),
              ]}
              searchPlaceholder="Filter platforms..."
            />
          </CardContent>
        </Card>
      </div>
    </PermissionGate>
  );
}
