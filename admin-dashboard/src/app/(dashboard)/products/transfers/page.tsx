"use client";

import Link from "next/link";
import { Fragment, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ArrowLeftRight, ChevronDown, ChevronRight } from "lucide-react";
import { PageHeader } from "@/components/dashboard-blocks";
import { Button, Card, CardContent } from "@/components/ui/primitives";
import { api } from "@/lib/api-client";

const badgeTone: Record<string, string> = {
    draft: "bg-[var(--muted)] text-[var(--foreground)]",
    pending: "bg-blue-500/15 text-blue-700 dark:text-blue-300",
    in_transit: "bg-amber-500/15 text-amber-700 dark:text-amber-300",
    completed: "bg-emerald-500/15 text-emerald-700 dark:text-emerald-300",
    cancelled: "bg-rose-500/15 text-rose-700 dark:text-rose-300",
};

export default function TransfersPage() {
    const [expandedTransferIDs, setExpandedTransferIDs] = useState<string[]>([]);
    const { data } = useQuery({
        queryKey: ["transfers"],
        queryFn: () => api.listTransfers(),
    });
    const { data: warehouses = [] } = useQuery({
        queryKey: ["warehouses"],
        queryFn: api.listWarehouses,
    });
    const { data: productsData } = useQuery({
        queryKey: ["products-for-transfer-list-preview"],
        queryFn: () => api.listProducts({ page: 1, page_size: 500 }),
    });
    const transfers = data?.items ?? [];
    const warehouseNameByID = useMemo(() => {
        const map = new Map<string, string>();
        for (const w of warehouses) map.set(w.id, w.name);
        return map;
    }, [warehouses]);
    const productThumbByID = useMemo(() => {
        const map = new Map<string, string | null | undefined>();
        for (const p of productsData?.items ?? []) map.set(p.product_id, p.thumbnail);
        return map;
    }, [productsData?.items]);

    return (
        <div className="space-y-6">
            <PageHeader
                title="Transfers"
                description="Move stock between warehouses."
            />
            <div className="flex justify-end">
                <Button asChild>
                    <Link href="/products/transfers/new">Create Transfer</Link>
                </Button>
            </div>
            {transfers.length === 0 ? (
                <div className="flex min-h-[360px] flex-col items-center justify-center rounded-2xl border border-dashed border-[var(--border)] bg-[var(--panel)] text-center px-8 py-16">
                    <div className="mb-6 flex size-16 items-center justify-center rounded-2xl bg-[var(--muted)]">
                        <ArrowLeftRight className="size-8 text-[var(--muted-foreground)]" />
                    </div>
                    <h2 className="mb-2 text-lg font-semibold">No transfers yet</h2>
                    <p className="mb-8 max-w-sm text-sm text-[var(--muted-foreground)]">
                        Move inventory between warehouses to keep stock balanced.
                    </p>
                    <Button asChild>
                        <Link href="/products/transfers/new">Create Transfer</Link>
                    </Button>
                </div>
            ) : (
                <Card>
                    <CardContent className="overflow-x-auto pt-5">
                        <table className="min-w-full text-sm">
                            <thead className="text-left text-xs text-[var(--muted-foreground)]">
                                <tr>
                                    <th className="pb-2 w-10"></th>
                                    <th className="pb-2">Transfer ID / Reference</th>
                                    <th className="pb-2">Origin</th>
                                    <th className="pb-2">Destination</th>
                                    <th className="pb-2">Total Items</th>
                                    <th className="pb-2">Status</th>
                                    <th className="pb-2">Date Created</th>
                                    <th className="pb-2">Actions</th>
                                </tr>
                            </thead>
                            <tbody>
                                {transfers.map((tr) => {
                                    const expanded = expandedTransferIDs.includes(tr.id);
                                    return (
                                        <Fragment key={tr.id}>
                                            <tr className="border-t border-[var(--border)]">
                                                <td className="py-3">
                                                    <button
                                                        type="button"
                                                        className="inline-flex h-7 w-7 items-center justify-center rounded-md border border-[var(--border)] hover:bg-[var(--muted)]"
                                                        onClick={() =>
                                                            setExpandedTransferIDs((prev) =>
                                                                prev.includes(tr.id) ? prev.filter((id) => id !== tr.id) : [...prev, tr.id],
                                                            )
                                                        }
                                                        aria-label={expanded ? "Collapse transfer items" : "Expand transfer items"}
                                                    >
                                                        {expanded ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
                                                    </button>
                                                </td>
                                                <td className="py-3">
                                                    <div className="font-medium">
                                                        {tr.reference?.trim() ? tr.reference : `TRF-${tr.id.slice(0, 8).toUpperCase()}`}
                                                    </div>
                                                    <div className="text-xs text-[var(--muted-foreground)]">#{tr.id.slice(0, 8)}</div>
                                                </td>
                                                <td className="py-3">{warehouseNameByID.get(tr.origin_warehouse_id) || "Unknown warehouse"}</td>
                                                <td className="py-3">{warehouseNameByID.get(tr.destination_warehouse_id) || "Unknown warehouse"}</td>
                                                <td className="py-3">{tr.total_items ?? tr.items?.length ?? 0}</td>
                                                <td className="py-3">
                                                    <span className={`rounded-full px-2 py-0.5 text-xs ${badgeTone[tr.status] || "bg-slate-100"}`}>
                                                        {tr.status.replaceAll("_", " ").replace(/\b\w/g, (c) => c.toUpperCase())}
                                                    </span>
                                                </td>
                                                <td className="py-3 text-xs">{tr.created_at ? new Date(tr.created_at).toLocaleString(undefined, { year: "numeric", month: "short", day: "2-digit", hour: "2-digit", minute: "2-digit" }) : "-"}</td>
                                                <td className="py-3">
                                                    <Button size="sm" variant="outline" asChild>
                                                        <Link href={`/products/transfers/${tr.id}`}>View / Edit</Link>
                                                    </Button>
                                                </td>
                                            </tr>
                                            {expanded ? (
                                                <tr className="border-t border-[var(--border)] bg-[var(--muted)]/20">
                                                    <td colSpan={8} className="p-3">
                                                        <TransferItemsPreview
                                                            transferID={tr.id}
                                                            originWarehouseID={tr.origin_warehouse_id}
                                                            productThumbByID={productThumbByID}
                                                        />
                                                    </td>
                                                </tr>
                                            ) : null}
                                        </Fragment>
                                    );
                                })}
                            </tbody>
                        </table>
                    </CardContent>
                </Card>
            )}
        </div>
    );
}

function TransferItemsPreview({
    transferID,
    originWarehouseID,
    productThumbByID,
}: {
    transferID: string;
    originWarehouseID: string;
    productThumbByID: Map<string, string | null | undefined>;
}) {
    const { data: transfer } = useQuery({
        queryKey: ["transfer", transferID, "preview"],
        queryFn: () => api.getTransfer(transferID),
    });
    const { data: inventory } = useQuery({
        queryKey: ["inventory", originWarehouseID, "transfer-preview"],
        queryFn: () => api.listInventoryRows({ warehouse_id: originWarehouseID }),
    });

    const variantMeta = useMemo(() => {
        const map = new Map<string, { productID: string; product: string; variant: string; sku: string }>();
        for (const row of inventory?.items ?? []) {
            map.set(row.variant_id, {
                productID: row.product_id,
                product: row.product_name || "Unknown product",
                variant: row.variant_name || "Default variant",
                sku: row.sku || "-",
            });
        }
        return map;
    }, [inventory?.items]);

    const grouped = useMemo(() => {
        const map = new Map<string, { productID: string; product: string; rows: Array<{ variant: string; sku: string; qty: number }> }>();
        for (const item of transfer?.items ?? []) {
            const meta = variantMeta.get(item.variant_id);
            const key = meta?.product || "Unknown product";
            const existing = map.get(key) ?? { productID: meta?.productID || "", product: key, rows: [] };
            existing.rows.push({
                variant: meta?.variant || item.variant_id,
                sku: meta?.sku || "-",
                qty: item.quantity,
            });
            map.set(key, existing);
        }
        return Array.from(map.values());
    }, [transfer?.items, variantMeta]);

    if (!transfer) return <div className="text-xs text-[var(--muted-foreground)]">Loading items...</div>;
    if (grouped.length === 0) return <div className="text-xs text-[var(--muted-foreground)]">No items in this transfer.</div>;

    return (
        <div className="space-y-3">
            {grouped.map((group) => (
                <div key={group.product + group.productID} className="rounded-xl border border-[var(--border)] bg-[var(--panel)] p-3">
                    <div className="mb-2 flex items-center gap-2">
                        {productThumbByID.get(group.productID) ? (
                            // eslint-disable-next-line @next/next/no-img-element
                            <img
                                src={productThumbByID.get(group.productID) || ""}
                                alt={group.product}
                                className="h-9 w-9 rounded-md object-cover"
                            />
                        ) : (
                            <div className="h-9 w-9 rounded-md bg-[var(--muted)]" />
                        )}
                        <div className="font-medium">{group.product}</div>
                    </div>
                    <table className="min-w-full text-sm">
                        <thead className="text-left text-xs text-[var(--muted-foreground)]">
                            <tr>
                                <th className="py-1">Variant</th>
                                <th className="py-1">SKU</th>
                                <th className="py-1">Qty</th>
                            </tr>
                        </thead>
                        <tbody>
                            {group.rows.map((row, idx) => (
                                <tr key={`${group.product}-${row.sku}-${idx}`} className="border-t border-[var(--border)]">
                                    <td className="py-1.5">{row.variant}</td>
                                    <td className="py-1.5">{row.sku}</td>
                                    <td className="py-1.5">{row.qty}</td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                </div>
            ))}
        </div>
    );
}
