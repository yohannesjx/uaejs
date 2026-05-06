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
import { formatCurrency } from "@/lib/utils";
import type { OrderListItem } from "@/types/api";

const orderLookupSchema = z.object({
  orderId: z.string().uuid(),
});

const col = createColumnHelper<OrderListItem>();

const orderColumns = [
  col.accessor("id", {
    header: "Order",
    cell: (c) => (
      <Link
        href={`/orders/${c.getValue()}`}
        className="font-mono text-xs text-[var(--primary)] hover:underline"
      >
        {String(c.getValue()).slice(0, 8)}…
      </Link>
    ),
  }),
  col.accessor("channel_name", { header: "Channel", cell: (c) => c.getValue() }),
  col.accessor("customer_name", {
    header: "Customer",
    cell: (c) => c.getValue() ?? "—",
  }),
  col.accessor("total_amount", {
    header: "Total",
    cell: (c) => formatCurrency(c.getValue(), c.row.original.currency),
  }),
  col.accessor("status", {
    header: "Status",
    cell: (c) => c.getValue(),
  }),
  col.accessor("payment_status", {
    header: "Payment",
    cell: (c) => c.getValue(),
  }),
  col.accessor("created_at", {
    header: "Date",
    cell: (c) =>
      c.getValue()
        ? new Date(c.getValue() as string).toLocaleDateString()
        : "—",
  }),
];

export default function OrdersPage() {
  const [page, setPage] = useState(1);
  const pageSize = 25;

  const { data, isLoading } = useQuery({
    queryKey: ["orders", page, pageSize],
    queryFn: () =>
      api.listOrders({
        page,
        page_size: pageSize,
      }),
  });

  const orderLookup = useForm<z.infer<typeof orderLookupSchema>>({
    resolver: zodResolver(orderLookupSchema),
    defaultValues: { orderId: "" },
  });

  const invoiceMutation = useMutation({
    mutationFn: async (values: z.infer<typeof orderLookupSchema>) =>
      api.getInvoiceXml(values.orderId),
  });

  return (
    <div className="space-y-8">
      <PageHeader
        title="Orders & compliance"
        description="Order list, invoice XML preview, and compliance tools."
      />

      <div className="grid gap-6 xl:grid-cols-[0.5fr_1.5fr]">
        <Card>
          <CardHeader>
            <CardTitle>Invoice XML lookup</CardTitle>
            <CardDescription>
              Pulls the generated invoice/receipt XML for an order by ID.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form
              className="space-y-3"
              onSubmit={orderLookup.handleSubmit((values) =>
                invoiceMutation.mutate({ orderId: values.orderId }),
              )}
            >
              <div className="space-y-2">
                <Label>Order ID</Label>
                <Input
                  {...orderLookup.register("orderId")}
                  placeholder="UUID"
                />
              </div>
              <Button
                type="submit"
                className="w-full"
                loading={invoiceMutation.isPending}
              >
                Load invoice XML
              </Button>
            </form>

            {invoiceMutation.data ? (
              <Textarea
                readOnly
                className="mt-4 min-h-[320px] font-mono text-xs"
                value={invoiceMutation.data}
              />
            ) : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Orders</CardTitle>
            <CardDescription>
              {data?.total ?? 0} orders · server-paginated
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ServerDataTable<OrderListItem>
              data={data?.items ?? []}
              total={data?.total ?? 0}
              page={page}
              pageSize={pageSize}
              onPageChange={setPage}
              columns={orderColumns}
              isLoading={isLoading}
            />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
