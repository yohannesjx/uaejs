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
} from "@/components/ui/primitives";
import { api } from "@/lib/api-client";
import type { ShipmentListItem } from "@/types/api";

const trackingSchema = z.object({
  shipmentId: z.string().uuid(),
});

const col = createColumnHelper<ShipmentListItem>();

const shipmentColumns = [
  col.accessor("id", {
    header: "Shipment",
    cell: (c) => (
      <Link
        href={`/shipments/${c.getValue()}`}
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
  col.accessor("tracking_number", {
    header: "Tracking",
    cell: (c) => c.getValue() ?? "—",
  }),
  col.accessor("carrier", {
    header: "Carrier",
    cell: (c) => c.getValue() ?? "—",
  }),
  col.accessor("status", { header: "Status", cell: (c) => c.getValue() }),
  col.accessor("created_at", {
    header: "Created",
    cell: (c) =>
      c.getValue()
        ? new Date(c.getValue() as string).toLocaleDateString()
        : "—",
  }),
];

export default function ShipmentsPage() {
  const [page, setPage] = useState(1);
  const pageSize = 25;

  const { data, isLoading } = useQuery({
    queryKey: ["shipments", page, pageSize],
    queryFn: () =>
      api.listShipments({
        page,
        page_size: pageSize,
      }),
  });

  const form = useForm<z.infer<typeof trackingSchema>>({
    resolver: zodResolver(trackingSchema),
    defaultValues: { shipmentId: "" },
  });

  const trackingMutation = useMutation({
    mutationFn: (values: z.infer<typeof trackingSchema>) =>
      api.getShipmentTracking(values.shipmentId),
  });

  return (
    <div className="space-y-8">
      <PageHeader
        title="Shipping"
        description="Carrier tracking lookup and shipment monitoring."
      />

      <div className="grid gap-6 xl:grid-cols-[0.5fr_1.5fr]">
        <Card>
          <CardHeader>
            <CardTitle>Tracking lookup</CardTitle>
            <CardDescription>
              Load shipment events by shipment ID.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form
              className="space-y-3"
              onSubmit={form.handleSubmit((values) =>
                trackingMutation.mutate(values),
              )}
            >
              <div className="space-y-2">
                <Label>Shipment ID</Label>
                <Input
                  {...form.register("shipmentId")}
                  placeholder="UUID"
                />
              </div>
              <Button
                type="submit"
                className="w-full"
                loading={trackingMutation.isPending}
              >
                Load tracking events
              </Button>
            </form>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Shipments</CardTitle>
            <CardDescription>
              {data?.total ?? 0} shipments · server-paginated
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ServerDataTable<ShipmentListItem>
              data={data?.items ?? []}
              total={data?.total ?? 0}
              page={page}
              pageSize={pageSize}
              onPageChange={setPage}
              columns={shipmentColumns}
              isLoading={isLoading}
            />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
