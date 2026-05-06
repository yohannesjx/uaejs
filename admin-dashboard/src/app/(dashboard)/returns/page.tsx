"use client";

import { createColumnHelper } from "@tanstack/react-table";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation } from "@tanstack/react-query";
import Link from "next/link";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { ServerDataTable } from "@/components/server-data-table";
import { PageHeader } from "@/components/dashboard-blocks";
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
  Label,
  Textarea,
} from "@/components/ui/primitives";
import { api } from "@/lib/api-client";
import type { ReturnListItem } from "@/types/api";

const lookupSchema = z.object({
  returnId: z.string().uuid(),
});

const col = createColumnHelper<ReturnListItem>();

const returnColumns = [
  col.accessor("id", {
    header: "Return",
    cell: (c) => (
      <Link
        href={`/returns/${c.getValue()}`}
        className="font-mono text-xs text-[var(--primary)] hover:underline"
      >
        {String(c.getValue()).slice(0, 8)}…
      </Link>
    ),
  }),
  col.accessor("order_id", {
    header: "Order",
    cell: (c) => (
      <span className="font-mono text-xs">
        {String(c.getValue()).slice(0, 8)}…
      </span>
    ),
  }),
  col.accessor("customer_name", {
    header: "Customer",
    cell: (c) => c.getValue(),
  }),
  col.accessor("status", { header: "Status", cell: (c) => c.getValue() }),
  col.accessor("item_count", {
    header: "Items",
    cell: (c) => c.getValue(),
  }),
  col.accessor("return_reason", {
    header: "Reason",
    cell: (c) => c.getValue(),
  }),
  col.accessor("requested_at", {
    header: "Requested",
    cell: (c) =>
      c.getValue()
        ? new Date(c.getValue() as string).toLocaleDateString()
        : "—",
  }),
];

export default function ReturnsPage() {
  const [page, setPage] = useState(1);
  const pageSize = 25;

  const { data, isLoading } = useQuery({
    queryKey: ["returns", page, pageSize],
    queryFn: () =>
      api.listReturns({
        page,
        page_size: pageSize,
      }),
  });

  const form = useForm<z.infer<typeof lookupSchema>>({
    resolver: zodResolver(lookupSchema),
    defaultValues: { returnId: "" },
  });

  const lookupMutation = useMutation({
    mutationFn: (values: z.infer<typeof lookupSchema>) =>
      api.getReturn(values.returnId),
  });

  return (
    <div className="space-y-8">
      <PageHeader
        title="Returns / RMA"
        description="Inspect return requests, QC metadata, and approval queues."
      />

      <div className="grid gap-6 xl:grid-cols-[0.5fr_1.5fr]">
        <Card>
          <CardHeader>
            <CardTitle>Lookup return</CardTitle>
            <CardDescription>
              Load an RMA record by ID using the detail endpoint.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form
              className="space-y-3"
              onSubmit={form.handleSubmit((values) =>
                lookupMutation.mutate(values),
              )}
            >
              <div className="space-y-2">
                <Label>Return ID</Label>
                <Input {...form.register("returnId")} placeholder="UUID" />
              </div>
              <Button
                type="submit"
                className="w-full"
                loading={lookupMutation.isPending}
              >
                Load return
              </Button>
            </form>

            {lookupMutation.data ? (
              <Textarea
                readOnly
                className="mt-4 min-h-[320px] font-mono text-xs"
                value={JSON.stringify(lookupMutation.data, null, 2)}
              />
            ) : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Returns queue</CardTitle>
            <CardDescription>
              {data?.total ?? 0} returns · server-paginated
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ServerDataTable<ReturnListItem>
              data={data?.items ?? []}
              total={data?.total ?? 0}
              page={page}
              pageSize={pageSize}
              onPageChange={setPage}
              columns={returnColumns}
              isLoading={isLoading}
            />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
