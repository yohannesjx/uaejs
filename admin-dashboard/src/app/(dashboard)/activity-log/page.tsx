"use client";

import { createColumnHelper } from "@tanstack/react-table";
import { useQuery } from "@tanstack/react-query";
import { useCallback, useState } from "react";

import { ServerDataTable } from "@/components/server-data-table";
import { PageHeader } from "@/components/dashboard-blocks";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/primitives";
import { api } from "@/lib/api-client";
import { formatDate } from "@/lib/utils";
import type { ActivityLogItem } from "@/types/api";

const col = createColumnHelper<ActivityLogItem>();

const activityColumns = [
  col.accessor("event_type", {
    header: "Event",
    cell: (c) => (
      <span className="rounded-md bg-[var(--muted)] px-2 py-0.5 text-xs font-medium">
        {c.getValue()}
      </span>
    ),
  }),
  col.accessor("title", { header: "Title", cell: (c) => c.getValue() }),
  col.accessor("description", {
    header: "Description",
    cell: (c) => {
      const v = c.getValue();
      return v ? <span className="text-sm text-[var(--muted-foreground)]">{v}</span> : "—";
    },
  }),
  col.accessor("actor", {
    header: "Actor",
    cell: (c) => c.getValue(),
  }),
  col.accessor("subject_type", {
    header: "Subject",
    cell: (c) => `${c.getValue()} ${c.row.original.subject_id ? `#${String(c.row.original.subject_id).slice(0, 8)}` : ""}`,
  }),
  col.accessor("created_at", {
    header: "When",
    cell: (c) => formatDate(c.getValue()),
  }),
];

export default function ActivityLogPage() {
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const pageSize = 25;

  const { data, isLoading } = useQuery({
    queryKey: ["activity-log", page, pageSize, search],
    queryFn: () =>
      api.listActivityLog({
        page,
        page_size: pageSize,
        search: search || undefined,
      }),
  });

  const handleSearchChange = useCallback((s: string) => {
    setSearch(s);
    setPage(1);
  }, []);

  return (
    <div className="space-y-8">
      <PageHeader
        title="Activity Log"
        description="Audit trail of admin actions: warehouse transfers, return approvals, product updates, and more."
      />

      <Card>
        <CardHeader>
          <CardTitle>Recent activity</CardTitle>
          <CardDescription>
            {data?.total ?? 0} events · server-paginated
          </CardDescription>
        </CardHeader>
        <CardContent>
          <ServerDataTable<ActivityLogItem>
            data={data?.items ?? []}
            total={data?.total ?? 0}
            page={page}
            pageSize={pageSize}
            onPageChange={setPage}
            onSearchChange={handleSearchChange}
            searchPlaceholder="Search by title, actor, or description..."
            columns={activityColumns}
            isLoading={isLoading}
          />
        </CardContent>
      </Card>
    </div>
  );
}
