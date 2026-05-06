"use client";

import { useQuery } from "@tanstack/react-query";
import { createColumnHelper } from "@tanstack/react-table";

import { DataTable } from "@/components/data-table";
import { PageHeader } from "@/components/dashboard-blocks";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/primitives";
import { api } from "@/lib/api-client";
import type { WarehouseStock } from "@/types/api";

const stockColumn = createColumnHelper<WarehouseStock>();

export default function WarehouseInventoryPage({
  params,
}: {
  params: { id: string };
}) {
  const inventoryQuery = useQuery({
    queryKey: ["warehouse-inventory", params.id],
    queryFn: () => api.getWarehouseInventory(params.id),
    enabled: Boolean(params.id),
  });

  return (
    <div className="space-y-6">
      <PageHeader
        title="Warehouse inventory"
        description="Variant-level stock for the selected warehouse location."
      />

      <Card>
        <CardHeader>
          <CardTitle>Location stock map</CardTitle>
          <CardDescription>
            Per-location inventory counters exposed by <code>/admin/warehouses/{params.id}/inventory</code>.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            data={inventoryQuery.data ?? []}
            columns={[
              stockColumn.accessor("variant_id", { header: "Variant ID" }),
              stockColumn.accessor("qty_on_hand", { header: "On hand" }),
              stockColumn.accessor("qty_reserved", { header: "Reserved" }),
              stockColumn.accessor("qty_available", { header: "Available" }),
              stockColumn.accessor("reorder_point", { header: "Reorder point" }),
              stockColumn.accessor("reorder_qty", { header: "Reorder qty" }),
            ]}
            searchPlaceholder="Filter variant inventory..."
          />
        </CardContent>
      </Card>
    </div>
  );
}
