"use client";

import { useQuery } from "@tanstack/react-query";
import { createColumnHelper } from "@tanstack/react-table";

import { PermissionGate } from "@/components/permission-gate";
import { DataTable } from "@/components/data-table";
import { PageHeader } from "@/components/dashboard-blocks";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/primitives";
import { api } from "@/lib/api-client";
import { formatDate } from "@/lib/utils";
import type { Supplier } from "@/types/api";

const supplierColumn = createColumnHelper<Supplier>();

export default function SuppliersPage() {
  const suppliersQuery = useQuery({
    queryKey: ["suppliers"],
    queryFn: api.listSuppliers,
  });

  return (
    <PermissionGate
      permission="suppliers.manage"
      fallback={<div className="text-sm text-[var(--muted-foreground)]">Missing suppliers.manage permission.</div>}
    >
      <div className="space-y-6">
        <PageHeader
          title="Suppliers"
          description="Vendor management and procurement-facing data."
        />
        <Card>
          <CardHeader>
            <CardTitle>Supplier directory</CardTitle>
            <CardDescription>Live supplier list from <code>/admin/suppliers</code>.</CardDescription>
          </CardHeader>
          <CardContent>
            <DataTable
              data={suppliersQuery.data ?? []}
              columns={[
                supplierColumn.accessor("name", { header: "Supplier" }),
                supplierColumn.accessor("country", { header: "Country" }),
                supplierColumn.accessor("lead_time_days", { header: "Lead time" }),
                supplierColumn.accessor("minimum_order_qty", { header: "MOQ" }),
                supplierColumn.accessor("updated_at", {
                  header: "Updated",
                  cell: ({ getValue }) => formatDate(getValue()),
                }),
              ]}
              searchPlaceholder="Filter suppliers..."
            />
          </CardContent>
        </Card>
      </div>
    </PermissionGate>
  );
}
