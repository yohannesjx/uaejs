"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createColumnHelper } from "@tanstack/react-table";
import Link from "next/link";
import { useForm } from "react-hook-form";
import { toast } from "sonner";
import { z } from "zod";

import { DataTable } from "@/components/data-table";
import { PageHeader } from "@/components/dashboard-blocks";
import { Button, Card, CardContent, CardDescription, CardHeader, CardTitle, Input, Label, Textarea } from "@/components/ui/primitives";
import { api } from "@/lib/api-client";
import { formatDate } from "@/lib/utils";
import type { Warehouse } from "@/types/api";

const warehouseColumn = createColumnHelper<Warehouse>();

const createWarehouseSchema = z.object({
  name: z.string().min(2),
  type: z.enum(["warehouse", "store", "dropship", "virtual"]),
  address: z.string(),
  city: z.string(),
  country: z.string().min(2),
  priority: z.string().min(1),
});

const transferSchema = z.object({
  from_warehouse_id: z.string().uuid(),
  to_warehouse_id: z.string().uuid(),
  variant_id: z.string().uuid(),
  quantity: z.string().min(1),
  notes: z.string(),
});

type CreateWarehouseValues = z.infer<typeof createWarehouseSchema>;
type TransferValues = z.infer<typeof transferSchema>;

export default function WarehousesPage() {
  const queryClient = useQueryClient();

  const warehousesQuery = useQuery({
    queryKey: ["warehouses"],
    queryFn: api.listWarehouses,
  });

  const createForm = useForm<CreateWarehouseValues>({
    resolver: zodResolver(createWarehouseSchema),
    defaultValues: {
      name: "",
      type: "warehouse",
      address: "",
      city: "Dubai",
      country: "AE",
      priority: "100",
    },
  });

  const transferForm = useForm<TransferValues>({
    resolver: zodResolver(transferSchema),
    defaultValues: {
      from_warehouse_id: "",
      to_warehouse_id: "",
      variant_id: "",
      quantity: "1",
      notes: "",
    },
  });

  const createWarehouseMutation = useMutation({
    mutationFn: api.createWarehouse,
    onSuccess: async () => {
      createForm.reset();
      await queryClient.invalidateQueries({ queryKey: ["warehouses"] });
      toast.success("Warehouse created");
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : "Failed to create warehouse"),
  });

  const transferMutation = useMutation({
    mutationFn: api.transferWarehouseStock,
    onSuccess: () => {
      transferForm.reset();
      queryClient.invalidateQueries({ queryKey: ["warehouses"] });
      toast.success("Stock transferred");
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : "Transfer failed"),
  });

  return (
    <div className="space-y-8">
      <PageHeader
        title="Warehouses & transfers"
        description="Manage location inventory, fulfillment priority, and stock transfers."
      />

      <div className="grid gap-6 xl:grid-cols-[1.4fr_0.6fr]">
        <Card>
          <CardHeader>
            <CardTitle>Locations</CardTitle>
            <CardDescription>
              All tenant-visible warehouses and store locations protected by
              <code className="ml-1">inventory.manage</code>.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <DataTable
              data={warehousesQuery.data ?? []}
              columns={[
                warehouseColumn.display({
                  id: "name",
                  header: "Warehouse",
                  cell: ({ row }) => (
                    <div>
                      <p className="font-medium">{row.original.name}</p>
                      <p className="text-xs text-[var(--muted-foreground)]">
                        {row.original.city}, {row.original.country}
                      </p>
                    </div>
                  ),
                }),
                warehouseColumn.accessor("type", { header: "Type" }),
                warehouseColumn.accessor("priority", { header: "Priority" }),
                warehouseColumn.accessor("is_active", {
                  header: "Status",
                  cell: ({ getValue }) => (getValue() ? "Active" : "Inactive"),
                }),
                warehouseColumn.accessor("updated_at", {
                  header: "Updated",
                  cell: ({ getValue }) => formatDate(getValue()),
                }),
                warehouseColumn.display({
                  id: "actions",
                  header: "Open",
                  cell: ({ row }) => (
                    <Button asChild variant="outline" size="sm">
                      <Link href={`/warehouses/${row.original.id}`}>Inventory</Link>
                    </Button>
                  ),
                }),
              ]}
              searchPlaceholder="Search warehouses..."
            />
          </CardContent>
        </Card>

        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Create warehouse</CardTitle>
              <CardDescription>Add a new store, warehouse, or drop-ship node.</CardDescription>
            </CardHeader>
            <CardContent>
              <form
                className="space-y-3"
                onSubmit={createForm.handleSubmit((values) =>
                  createWarehouseMutation.mutate({
                    ...values,
                    priority: Number(values.priority),
                  }),
                )}
              >
                <div className="space-y-2">
                  <Label>Name</Label>
                  <Input {...createForm.register("name")} placeholder="Main Warehouse" />
                </div>
                <div className="space-y-2">
                  <Label>Type</Label>
                  <Input {...createForm.register("type")} placeholder="warehouse" />
                </div>
                <div className="space-y-2">
                  <Label>Address</Label>
                  <Textarea {...createForm.register("address")} placeholder="Al Quoz Industrial Area 3" />
                </div>
                <div className="grid grid-cols-2 gap-3">
                  <div className="space-y-2">
                    <Label>City</Label>
                    <Input {...createForm.register("city")} />
                  </div>
                  <div className="space-y-2">
                    <Label>Priority</Label>
                  <Input type="number" {...createForm.register("priority")} />
                  </div>
                </div>
                <Button type="submit" className="w-full" loading={createWarehouseMutation.isPending}>
                  Create location
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Transfer stock</CardTitle>
              <CardDescription>
                Executes <code>/admin/warehouses/transfer</code> with serializable stock protection.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form
                className="space-y-3"
                onSubmit={transferForm.handleSubmit((values) =>
                  transferMutation.mutate({
                    ...values,
                    quantity: Number(values.quantity),
                  }),
                )}
              >
                <div className="space-y-2">
                  <Label>From warehouse</Label>
                  <Input {...transferForm.register("from_warehouse_id")} placeholder="UUID" />
                </div>
                <div className="space-y-2">
                  <Label>To warehouse</Label>
                  <Input {...transferForm.register("to_warehouse_id")} placeholder="UUID" />
                </div>
                <div className="space-y-2">
                  <Label>Variant ID</Label>
                  <Input {...transferForm.register("variant_id")} placeholder="Variant UUID" />
                </div>
                <div className="space-y-2">
                  <Label>Quantity</Label>
                  <Input type="number" {...transferForm.register("quantity")} />
                </div>
                <div className="space-y-2">
                  <Label>Notes</Label>
                  <Textarea {...transferForm.register("notes")} placeholder="Urgent replenishment" />
                </div>
                <Button type="submit" className="w-full" loading={transferMutation.isPending}>
                  Transfer stock
                </Button>
                {transferMutation.data ? (
                  <p className="text-xs text-[var(--muted-foreground)]">
                    Transfer complete. Source available:{" "}
                    {transferMutation.data.from_stock.qty_available}, destination
                    available: {transferMutation.data.to_stock.qty_available}
                  </p>
                ) : null}
              </form>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
