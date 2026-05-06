"use client";

import { useParams } from "next/navigation";
import { useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { PageHeader } from "@/components/dashboard-blocks";
import { Button, Card, CardContent, CardHeader, CardTitle } from "@/components/ui/primitives";
import { api } from "@/lib/api-client";

export default function TransferDetailPage() {
    const params = useParams<{ id: string }>();
    const id = params.id;
    const qc = useQueryClient();
    const { data: transfer } = useQuery({
        queryKey: ["transfer", id],
        queryFn: () => api.getTransfer(id),
    });
    const { data: warehouses = [] } = useQuery({
        queryKey: ["warehouses"],
        queryFn: api.listWarehouses,
    });
    const { data: originInventory } = useQuery({
        queryKey: ["inventory", transfer?.origin_warehouse_id],
        enabled: Boolean(transfer?.origin_warehouse_id),
        queryFn: () => api.listInventoryRows({ warehouse_id: transfer?.origin_warehouse_id }),
    });

    const transition = useMutation({
        mutationFn: (status: "draft" | "pending" | "in_transit" | "completed" | "cancelled") => api.transitionTransfer(id, status),
        onSuccess: async (_, status) => {
            await qc.invalidateQueries({ queryKey: ["transfer", id] });
            await qc.invalidateQueries({ queryKey: ["transfers"] });
            await qc.invalidateQueries({ queryKey: ["inventory"] });
            if (status === "completed") {
                window.location.href = "/products/transfers";
            }
        },
    });

    if (!transfer) return <div className="p-6 text-sm text-[var(--muted-foreground)]">Loading transfer...</div>;

    const warehouseNameByID = useMemo(() => {
        const map = new Map<string, string>();
        for (const w of warehouses) map.set(w.id, w.name);
        return map;
    }, [warehouses]);
    const variantMetaByID = useMemo(() => {
        const map = new Map<string, { product: string; variant: string; sku: string; available: number }>();
        for (const row of originInventory?.items ?? []) {
            if (!map.has(row.variant_id)) {
                map.set(row.variant_id, {
                    product: row.product_name || "Unnamed product",
                    variant: row.variant_name || "Default variant",
                    sku: row.sku || "-",
                    available: row.available_quantity,
                });
            }
        }
        return map;
    }, [originInventory?.items]);
    const canMarkReady = transfer.status === "draft";
    const canStart = transfer.status === "draft" || transfer.status === "pending";
    const canComplete = transfer.status === "draft" || transfer.status === "pending" || transfer.status === "in_transit";
    const canCancel = transfer.status !== "completed" && transfer.status !== "cancelled";

    return (
        <div className="space-y-6">
            <PageHeader title={`Transfer ${transfer.reference || transfer.id}`} description="View and progress transfer lifecycle." />
            <div className="flex flex-wrap gap-2">
                <Button variant="outline" onClick={() => transition.mutate("pending")} disabled={transition.isPending || !canMarkReady}>Mark as Ready</Button>
                <Button variant="outline" onClick={() => transition.mutate("in_transit")} disabled={transition.isPending || !canStart}>Start Transfer</Button>
                <Button onClick={() => transition.mutate("completed")} disabled={transition.isPending || !canComplete}>Complete Transfer</Button>
                <Button variant="danger" onClick={() => transition.mutate("cancelled")} disabled={transition.isPending || !canCancel}>Cancel Transfer</Button>
            </div>
            <Card>
                <CardHeader><CardTitle>Transfer Summary</CardTitle></CardHeader>
                <CardContent className="grid grid-cols-1 gap-2 text-sm md:grid-cols-2">
                    <div><span className="text-[var(--muted-foreground)]">Status:</span> {transfer.status}</div>
                    <div><span className="text-[var(--muted-foreground)]">Reference:</span> {transfer.reference || "-"}</div>
                    <div><span className="text-[var(--muted-foreground)]">Origin:</span> {warehouseNameByID.get(transfer.origin_warehouse_id) || transfer.origin_warehouse_id}</div>
                    <div><span className="text-[var(--muted-foreground)]">Destination:</span> {warehouseNameByID.get(transfer.destination_warehouse_id) || transfer.destination_warehouse_id}</div>
                    <div className="md:col-span-2"><span className="text-[var(--muted-foreground)]">Notes:</span> {transfer.notes || "-"}</div>
                </CardContent>
            </Card>
            <Card>
                <CardHeader><CardTitle>Items</CardTitle></CardHeader>
                <CardContent>
                    <table className="min-w-full text-sm">
                        <thead className="text-left text-xs text-[var(--muted-foreground)]">
                            <tr>
                                <th className="py-2">Product</th>
                                <th className="py-2">Variant</th>
                                <th className="py-2">SKU</th>
                                <th className="py-2">At Origin</th>
                                <th className="py-2">Transfer Qty</th>
                            </tr>
                        </thead>
                        <tbody>
                            {(transfer.items ?? []).map((it) => {
                                const meta = variantMetaByID.get(it.variant_id);
                                return (
                                    <tr key={it.id} className="border-t border-[var(--border)]">
                                        <td className="py-2">{meta?.product || "Unknown product"}</td>
                                        <td className="py-2">{meta?.variant || "Unknown variant"}</td>
                                        <td className="py-2">{meta?.sku || "-"}</td>
                                        <td className="py-2">{meta?.available ?? "-"}</td>
                                        <td className="py-2">{it.quantity}</td>
                                    </tr>
                                );
                            })}
                        </tbody>
                    </table>
                </CardContent>
            </Card>
        </div>
    );
}
