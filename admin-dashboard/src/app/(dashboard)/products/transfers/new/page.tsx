"use client";

import Link from "next/link";
import { Fragment } from "react";
import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Search, X } from "lucide-react";
import { PageHeader } from "@/components/dashboard-blocks";
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from "@/components/ui/primitives";
import { api, publicUploadUrl } from "@/lib/api-client";

type DraftItem = {
    variant_id: string;
    product_id: string;
    product_name: string;
    variant_name: string;
    sku: string;
    available: number;
    quantity: number;
    thumbnail?: string | null;
};

export default function CreateTransferPage() {
    const qc = useQueryClient();
    const [reference, setReference] = useState("");
    const [originWarehouseID, setOriginWarehouseID] = useState("");
    const [destinationWarehouseID, setDestinationWarehouseID] = useState("");
    const [notes, setNotes] = useState("");
    const [tagsInput, setTagsInput] = useState("");
    const [isPickerOpen, setIsPickerOpen] = useState(false);
    const [pickerSearch, setPickerSearch] = useState("");
    const [items, setItems] = useState<DraftItem[]>([]);
    const [pickerSelectedVariantIDs, setPickerSelectedVariantIDs] = useState<string[]>([]);

    const { data: warehouses = [] } = useQuery({ queryKey: ["warehouses"], queryFn: api.listWarehouses });
    const { data: productsPage } = useQuery({
        queryKey: ["products-for-transfer-picker"],
        queryFn: () => api.listProducts({ page: 1, page_size: 500 }),
    });
    const { data: inventory } = useQuery({
        queryKey: ["inventory", originWarehouseID, pickerSearch],
        enabled: Boolean(originWarehouseID),
        queryFn: () => api.listInventoryRows({
            warehouse_id: originWarehouseID || undefined,
            product: pickerSearch || undefined,
        }),
    });

    const candidates = inventory?.items ?? [];
    const groupedCandidates = useMemo(() => {
        const map = new Map<string, typeof candidates>();
        for (const row of candidates) {
            const key = `${row.product_id}::${row.product_name}`;
            const list = map.get(key) ?? [];
            list.push(row);
            map.set(key, list);
        }
        return Array.from(map.entries());
    }, [candidates]);
    const productThumbMap = useMemo(() => {
        const map = new Map<string, string | null | undefined>();
        for (const p of productsPage?.items ?? []) map.set(p.product_id, p.thumbnail);
        return map;
    }, [productsPage?.items]);

    const addVariant = (row: (typeof candidates)[number], thumbnail?: string | null) => {
        setItems((prev) => {
            if (prev.some((x) => x.variant_id === row.variant_id)) return prev;
            return [...prev, {
                variant_id: row.variant_id,
                product_id: row.product_id,
                product_name: row.product_name,
                variant_name: row.variant_name || "-",
                sku: row.sku,
                available: row.available_quantity,
                quantity: Math.min(1, Math.max(0, row.available_quantity)),
                thumbnail,
            }];
        });
    };

    const applyPickerSelection = () => {
        const selected = candidates.filter((x) => pickerSelectedVariantIDs.includes(x.variant_id));
        selected.forEach((row) => addVariant(row, productThumbMap.get(row.product_id)));
        setIsPickerOpen(false);
        setPickerSelectedVariantIDs([]);
    };

    const isWarehouseValid = originWarehouseID && destinationWarehouseID && originWarehouseID !== destinationWarehouseID;
    const hasItemErrors = useMemo(() => items.some((x) => x.quantity <= 0 || x.quantity > x.available), [items]);
    const canSubmit = isWarehouseValid && items.length > 0 && !hasItemErrors;
    const tags = tagsInput.split(",").map((x) => x.trim()).filter(Boolean).slice(0, 20).map((x) => x.slice(0, 40));

    const createTransfer = useMutation({
        mutationFn: () => api.createTransfer({
            reference,
            origin_warehouse_id: originWarehouseID,
            destination_warehouse_id: destinationWarehouseID,
            notes: notes || undefined,
            tags,
            status: "draft",
            items: items.map((x) => ({ variant_id: x.variant_id, quantity: x.quantity })),
        }),
        onSuccess: async () => {
            await qc.invalidateQueries({ queryKey: ["transfers"] });
            window.location.href = "/products/transfers";
        },
    });

    return (
        <div className="space-y-6">
            <PageHeader title="Create Transfer" description="Move variant inventory between warehouses." />
            <div className="flex gap-2">
                <Button onClick={() => createTransfer.mutate()} disabled={!canSubmit || createTransfer.isPending}>
                    Save Transfer
                </Button>
                <Button variant="ghost" asChild><Link href="/products/transfers">Cancel</Link></Button>
            </div>

            <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
                <div className="space-y-4 lg:col-span-2">
                    <Card>
                        <CardContent className="grid grid-cols-1 gap-3 pt-5 md:grid-cols-2">
                            <div>
                                <div className="mb-1 text-xs text-[var(--muted-foreground)]">Origin</div>
                                <select value={originWarehouseID} onChange={(e) => setOriginWarehouseID(e.target.value)} className="h-10 w-full rounded-xl border border-[var(--border)] px-3">
                                    <option value="">Shop location</option>
                                    {warehouses.map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
                                </select>
                            </div>
                            <div>
                                <div className="mb-1 text-xs text-[var(--muted-foreground)]">Destination</div>
                                <select value={destinationWarehouseID} onChange={(e) => setDestinationWarehouseID(e.target.value)} className="h-10 w-full rounded-xl border border-[var(--border)] px-3">
                                    <option value="">Shop location</option>
                                    {warehouses.map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
                                </select>
                            </div>
                            {originWarehouseID && destinationWarehouseID && originWarehouseID === destinationWarehouseID && (
                                <p className="text-xs text-red-600">Origin and destination cannot be the same.</p>
                            )}
                        </CardContent>
                    </Card>

                    <Card>
                        <CardHeader><CardTitle>Add products</CardTitle></CardHeader>
                        <CardContent className="space-y-3">
                            <button
                                type="button"
                                onClick={() => setIsPickerOpen(true)}
                                disabled={!originWarehouseID}
                                className="flex h-10 w-full items-center gap-2 rounded-xl border border-[var(--border)] px-3 text-left text-sm text-[var(--muted-foreground)] disabled:opacity-50"
                            >
                                <Search className="size-4" />
                                Search products
                            </button>
                            {!originWarehouseID && (
                                <p className="text-xs text-[var(--muted-foreground)]">Select origin warehouse before adding products.</p>
                            )}
                            <div className="overflow-x-auto rounded-xl border border-[var(--border)]">
                                <table className="min-w-full text-sm">
                                    <thead className="text-left text-xs text-[var(--muted-foreground)]">
                                        <tr>
                                            <th className="px-3 py-2">Products</th>
                                            <th className="px-3 py-2">SKU</th>
                                            <th className="px-3 py-2">At origin</th>
                                            <th className="px-3 py-2">Quantity</th>
                                            <th className="px-3 py-2"></th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {items.map((it) => {
                                            const invalid = it.quantity > it.available || it.quantity <= 0;
                                            return (
                                                <tr key={it.variant_id} className="border-t border-[var(--border)]">
                                                    <td className="px-3 py-2">
                                                        <div className="flex items-center gap-2">
                                                            {it.thumbnail ? (
                                                                // eslint-disable-next-line @next/next/no-img-element
                                                                <img src={publicUploadUrl(it.thumbnail)} alt={it.product_name} className="h-8 w-8 rounded-md object-cover" />
                                                            ) : (
                                                                <div className="h-8 w-8 rounded-md bg-[var(--muted)]" />
                                                            )}
                                                            <div>
                                                                <div className="font-medium">{it.product_name}</div>
                                                                <div className="text-xs text-[var(--muted-foreground)]">{it.variant_name}</div>
                                                            </div>
                                                        </div>
                                                    </td>
                                                    <td className="px-3 py-2">{it.sku}</td>
                                                    <td className="px-3 py-2">{it.available}</td>
                                                    <td className="px-3 py-2">
                                                        <Input
                                                            type="number"
                                                            min={1}
                                                            max={it.available}
                                                            value={it.quantity}
                                                            onChange={(e) => setItems((prev) => prev.map((x) => x.variant_id === it.variant_id ? { ...x, quantity: Number(e.target.value || 0) } : x))}
                                                            className={`h-9 max-w-24 ${invalid ? "border-red-500" : ""}`}
                                                        />
                                                        {invalid && <p className="mt-1 text-xs text-red-600">Exceeds availability.</p>}
                                                    </td>
                                                    <td className="px-3 py-2 text-right">
                                                        <button type="button" className="text-[var(--muted-foreground)] hover:text-[var(--foreground)]" onClick={() => setItems((prev) => prev.filter((x) => x.variant_id !== it.variant_id))}>
                                                            <X className="size-4" />
                                                        </button>
                                                    </td>
                                                </tr>
                                            );
                                        })}
                                        {items.length === 0 && (
                                            <tr>
                                                <td colSpan={5} className="px-3 py-5 text-center text-xs text-[var(--muted-foreground)]">
                                                    No products selected yet.
                                                </td>
                                            </tr>
                                        )}
                                    </tbody>
                                </table>
                            </div>
                        </CardContent>
                    </Card>
                </div>

                <div className="space-y-4">
                    <Card>
                        <CardHeader><CardTitle>Notes</CardTitle></CardHeader>
                        <CardContent>
                            <textarea className="min-h-24 w-full rounded-xl border border-[var(--border)] p-3 text-sm" placeholder="Add notes..." value={notes} onChange={(e) => setNotes(e.target.value)} />
                        </CardContent>
                    </Card>

                    <Card>
                        <CardHeader><CardTitle>Transfer details</CardTitle></CardHeader>
                        <CardContent className="space-y-3">
                            <div className="space-y-1">
                                <div className="text-xs text-[var(--muted-foreground)]">Date created</div>
                                <Input value={new Date().toISOString().slice(0, 10)} disabled />
                            </div>
                            <div className="space-y-1">
                                <div className="text-xs text-[var(--muted-foreground)]">Reference name</div>
                                <Input maxLength={255} value={reference} onChange={(e) => setReference(e.target.value)} />
                                <div className="text-right text-xs text-[var(--muted-foreground)]">{reference.length}/255</div>
                            </div>
                        </CardContent>
                    </Card>

                    <Card>
                        <CardHeader><CardTitle>Tags</CardTitle></CardHeader>
                        <CardContent className="space-y-1">
                            <Input maxLength={40} value={tagsInput} onChange={(e) => setTagsInput(e.target.value)} />
                            <div className="text-right text-xs text-[var(--muted-foreground)]">{Math.min(tagsInput.length, 40)}/40</div>
                        </CardContent>
                    </Card>
                </div>
            </div>

            {isPickerOpen && (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/30 p-4">
                    <div className="w-full max-w-4xl rounded-2xl border border-[var(--border)] bg-[var(--panel)] shadow-xl">
                        <div className="flex items-center justify-between border-b border-[var(--border)] p-4">
                            <h3 className="text-sm font-semibold">Select products</h3>
                            <button type="button" onClick={() => setIsPickerOpen(false)} className="text-[var(--muted-foreground)] hover:text-[var(--foreground)]">
                                <X className="size-4" />
                            </button>
                        </div>
                        <div className="space-y-3 p-4">
                            <Input
                                value={pickerSearch}
                                onChange={(e) => setPickerSearch(e.target.value)}
                                placeholder="Search by product name / SKU"
                            />
                            <div className="max-h-[420px] overflow-auto rounded-xl border border-[var(--border)]">
                                <table className="min-w-full text-sm">
                                    <thead className="text-left text-xs text-[var(--muted-foreground)]">
                                        <tr>
                                            <th className="px-3 py-2">Select</th>
                                            <th className="px-3 py-2">Product</th>
                                            <th className="px-3 py-2">Variant</th>
                                            <th className="px-3 py-2">SKU</th>
                                            <th className="px-3 py-2">Availability</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {groupedCandidates.map(([key, rows]) => (
                                            <Fragment key={key}>
                                                <tr key={`${key}-header`} className="border-t border-[var(--border)] bg-[var(--muted)]/40">
                                                    <td colSpan={5} className="px-3 py-2 text-xs font-semibold text-[var(--muted-foreground)]">
                                                        {rows[0]?.product_name}
                                                    </td>
                                                </tr>
                                                {rows.map((row) => {
                                                    const checked = pickerSelectedVariantIDs.includes(row.variant_id);
                                                    const thumb = productThumbMap.get(row.product_id);
                                                    return (
                                                        <tr key={`${row.variant_id}-${row.warehouse_id}`} className="border-t border-[var(--border)]">
                                                            <td className="px-3 py-2">
                                                                <input
                                                                    type="checkbox"
                                                                    checked={checked}
                                                                    onChange={(e) => {
                                                                        if (e.target.checked) {
                                                                            setPickerSelectedVariantIDs((prev) => [...prev, row.variant_id]);
                                                                        } else {
                                                                            setPickerSelectedVariantIDs((prev) => prev.filter((id) => id !== row.variant_id));
                                                                        }
                                                                    }}
                                                                />
                                                            </td>
                                                            <td className="px-3 py-2">
                                                                <div className="flex items-center gap-2">
                                                                    {thumb ? (
                                                                        // eslint-disable-next-line @next/next/no-img-element
                                                                        <img src={publicUploadUrl(thumb)} alt={row.product_name} className="h-8 w-8 rounded-md object-cover" />
                                                                    ) : (
                                                                        <div className="h-8 w-8 rounded-md bg-[var(--muted)]" />
                                                                    )}
                                                                    <span>{row.product_name}</span>
                                                                </div>
                                                            </td>
                                                            <td className="px-3 py-2">{row.variant_name || "Default variant"}</td>
                                                            <td className="px-3 py-2">{row.sku}</td>
                                                            <td className="px-3 py-2">Available: {row.available_quantity} units</td>
                                                        </tr>
                                                    );
                                                })}
                                            </Fragment>
                                        ))}
                                        {candidates.length === 0 && (
                                            <tr>
                                                <td colSpan={5} className="px-3 py-5 text-center text-xs text-[var(--muted-foreground)]">
                                                    No variants found for the selected origin warehouse.
                                                </td>
                                            </tr>
                                        )}
                                    </tbody>
                                </table>
                            </div>
                        </div>
                        <div className="flex items-center justify-end gap-2 border-t border-[var(--border)] p-4">
                            <Button variant="ghost" onClick={() => setIsPickerOpen(false)}>Cancel</Button>
                            <Button onClick={applyPickerSelection} disabled={pickerSelectedVariantIDs.length === 0}>Add Selected</Button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
