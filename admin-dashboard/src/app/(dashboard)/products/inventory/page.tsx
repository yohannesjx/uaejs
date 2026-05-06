"use client";

import { Fragment, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BarChart2, ArrowRightLeft, ChevronDown, ChevronRight } from "lucide-react";
import { api } from "@/lib/api-client";
import { PageHeader } from "@/components/dashboard-blocks";
import Link from "next/link";
import { Button, Card, CardContent, Input, Label } from "@/components/ui/primitives";

export default function ProductsInventoryPage() {
    const queryClient = useQueryClient();
    const [warehouseId, setWarehouseId] = useState("");
    const [productFilter, setProductFilter] = useState("");
    const [categoryFilter, setCategoryFilter] = useState("");
    const [lowStockOnly, setLowStockOnly] = useState(false);
    const [adjustOpen, setAdjustOpen] = useState(false);
    const [transferOpen, setTransferOpen] = useState(false);
    const [selectedVariant, setSelectedVariant] = useState<{ variant_id: string; warehouse_id: string } | null>(null);
    const [adjustType, setAdjustType] = useState<"increase" | "decrease">("increase");
    const [adjustQty, setAdjustQty] = useState("0");
    const [adjustReason, setAdjustReason] = useState("");
    const [transferTo, setTransferTo] = useState("");
    const [transferQty, setTransferQty] = useState("0");
    const [createWarehouseOpen, setCreateWarehouseOpen] = useState(false);
    const [warehouseName, setWarehouseName] = useState("");
    const [warehouseType, setWarehouseType] = useState("warehouse");
    const [warehouseCity, setWarehouseCity] = useState("Dubai");
    const [warehouseAddress, setWarehouseAddress] = useState("");
    const [warehousePriority, setWarehousePriority] = useState("100");
    const [expandedProductIDs, setExpandedProductIDs] = useState<string[]>([]);

    const { data: warehouses = [] } = useQuery({
        queryKey: ["warehouses"],
        queryFn: () => api.listWarehouses(),
    });
    const { data: inventoryResp, isLoading } = useQuery({
        queryKey: ["inventory-rows", warehouseId, productFilter, categoryFilter, lowStockOnly],
        queryFn: () => api.listInventoryRows({
            warehouse_id: warehouseId || undefined,
            product: productFilter || undefined,
            category: categoryFilter || undefined,
            low_stock: lowStockOnly,
        }),
    });
    const rows = inventoryResp?.items ?? [];
    const { data: productsData } = useQuery({
        queryKey: ["products-for-inventory-thumbs"],
        queryFn: () => api.listProducts({ page: 1, page_size: 500 }),
    });

    const adjustMutation = useMutation({
        mutationFn: () => {
            if (!selectedVariant) return Promise.resolve();
            return api.adjustInventory({
                warehouse_id: selectedVariant.warehouse_id,
                variant_id: selectedVariant.variant_id,
                adjustment_type: adjustType,
                quantity: Math.max(0, Number(adjustQty || 0)),
                reason: adjustReason || undefined,
            });
        },
        onSuccess: () => {
            setAdjustOpen(false);
            queryClient.invalidateQueries({ queryKey: ["inventory-rows"] });
        },
    });

    const transferMutation = useMutation({
        mutationFn: (): Promise<void> => {
            if (!selectedVariant || !transferTo) return Promise.resolve();
            return api.transferWarehouseStock({
                from_warehouse_id: selectedVariant.warehouse_id,
                to_warehouse_id: transferTo,
                variant_id: selectedVariant.variant_id,
                quantity: Math.max(0, Number(transferQty || 0)),
            }).then(() => undefined);
        },
        onSuccess: () => {
            setTransferOpen(false);
            queryClient.invalidateQueries({ queryKey: ["inventory-rows"] });
        },
    });
    const createWarehouseMutation = useMutation({
        mutationFn: () =>
            api.createWarehouse({
                name: warehouseName.trim(),
                type: warehouseType as "warehouse" | "store" | "dropship" | "virtual",
                city: warehouseCity.trim() || "Dubai",
                address: warehouseAddress.trim(),
                country: "AE",
                priority: Number(warehousePriority || 100),
            }),
        onSuccess: () => {
            setCreateWarehouseOpen(false);
            setWarehouseName("");
            setWarehouseAddress("");
            setWarehousePriority("100");
            queryClient.invalidateQueries({ queryKey: ["warehouses"] });
            queryClient.invalidateQueries({ queryKey: ["inventory-rows"] });
        },
    });

    const warehouseChoices = useMemo(() => warehouses.map((w) => ({ id: w.id, name: w.name })), [warehouses]);
    const productThumbByID = useMemo(() => {
        const map = new Map<string, string | null | undefined>();
        for (const p of productsData?.items ?? []) map.set(p.product_id, p.thumbnail);
        return map;
    }, [productsData?.items]);
    const groupedProducts = useMemo(() => {
        const map = new Map<string, typeof rows>();
        for (const row of rows) {
            const list = map.get(row.product_id) ?? [];
            list.push(row);
            map.set(row.product_id, list);
        }
        return Array.from(map.entries()).map(([productID, productRows]) => {
            const first = productRows[0];
            const totalAvailable = productRows.reduce((acc, curr) => acc + curr.available_quantity, 0);
            const variantsCount = new Set(productRows.map((x) => x.variant_id)).size;
            const warehousesCount = new Set(productRows.map((x) => x.warehouse_id)).size;
            return {
                productID,
                productName: first?.product_name || "Unknown product",
                variantsCount,
                warehousesCount,
                totalAvailable,
                rows: productRows,
                variantRows: Array.from(
                    productRows.reduce((variantMap, row) => {
                        const current = variantMap.get(row.variant_id) ?? {
                            variant_id: row.variant_id,
                            variant_name: row.variant_name || "-",
                            sku: row.sku || "-",
                            available_quantity: 0,
                            reserved_quantity: 0,
                            incoming_quantity: 0,
                            product_id: row.product_id,
                            warehouses: [] as Array<{ id: string; name: string }>,
                            actionRow: row,
                        };
                        current.available_quantity += row.available_quantity;
                        current.reserved_quantity += row.reserved_quantity;
                        current.incoming_quantity += row.incoming_quantity;
                        if (!current.warehouses.some((w) => w.id === row.warehouse_id)) {
                            current.warehouses.push({ id: row.warehouse_id, name: row.warehouse });
                        }
                        variantMap.set(row.variant_id, current);
                        return variantMap;
                    }, new Map<string, {
                        variant_id: string;
                        variant_name: string;
                        sku: string;
                        available_quantity: number;
                        reserved_quantity: number;
                        incoming_quantity: number;
                        product_id: string;
                        warehouses: Array<{ id: string; name: string }>;
                        actionRow: typeof productRows[number];
                    }>())
                ).map(([, v]) => v),
            };
        });
    }, [rows]);

    return (
        <div className="space-y-6">
            <PageHeader
                title="Inventory"
                description="Stock levels across all your warehouses."
            />

            {warehouses.length === 0 ? (
                <div className="flex min-h-[300px] flex-col items-center justify-center rounded-2xl border border-dashed border-[var(--border)] bg-[var(--panel)] text-center px-8 py-12">
                    <BarChart2 className="mb-4 size-10 text-[var(--muted-foreground)]" />
                    <h2 className="mb-2 text-lg font-semibold">No warehouses configured</h2>
                    <p className="text-sm text-[var(--muted-foreground)]">
                        Create your first warehouse to start tracking inventory by location.
                    </p>
                    <div className="mt-4 flex gap-2">
                        <Button onClick={() => setCreateWarehouseOpen(true)}>Create Warehouse</Button>
                        <Button variant="outline" asChild>
                            <Link href="/warehouses">Open Warehouses</Link>
                        </Button>
                    </div>
                </div>
            ) : (
                <>
                    <Card>
                        <CardContent className="flex items-center justify-between gap-3 pt-5">
                            <div className="text-sm text-[var(--muted-foreground)]">
                                {warehouses.length} warehouse location(s) configured.
                            </div>
                            <div className="flex gap-2">
                                <Button variant="outline" onClick={() => setCreateWarehouseOpen(true)}>
                                    Create Warehouse
                                </Button>
                                <Button variant="outline" asChild>
                                    <Link href="/warehouses">Manage Warehouses</Link>
                                </Button>
                            </div>
                        </CardContent>
                    </Card>
                    <Card>
                        <CardContent className="grid gap-3 pt-5 md:grid-cols-4">
                            <select value={warehouseId} onChange={(e) => setWarehouseId(e.target.value)} className="h-10 rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3 text-sm">
                                <option value="">All warehouses</option>
                                {warehouseChoices.map((w) => <option key={w.id} value={w.id}>{w.name}</option>)}
                            </select>
                            <Input placeholder="Filter product / SKU" value={productFilter} onChange={(e) => setProductFilter(e.target.value)} />
                            <Input placeholder="Filter category" value={categoryFilter} onChange={(e) => setCategoryFilter(e.target.value)} />
                            <label className="flex items-center gap-2 text-sm">
                                <input type="checkbox" checked={lowStockOnly} onChange={(e) => setLowStockOnly(e.target.checked)} />
                                Low stock only
                            </label>
                        </CardContent>
                    </Card>
                    <Card>
                        <CardContent className="overflow-x-auto pt-5">
                            <table className="min-w-full text-sm">
                                <thead className="text-left text-xs text-[var(--muted-foreground)]">
                                    <tr>
                                        <th className="pb-2 w-10"></th>
                                        <th className="pb-2">Product</th>
                                        <th className="pb-2">Variants</th>
                                        <th className="pb-2">Locations</th>
                                        <th className="pb-2">Total Available</th>
                                        <th className="pb-2">Actions</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {isLoading ? (
                                        <tr><td colSpan={6} className="py-6 text-center text-[var(--muted-foreground)]">Loading inventory...</td></tr>
                                    ) : groupedProducts.length === 0 ? (
                                        <tr><td colSpan={6} className="py-6 text-center text-[var(--muted-foreground)]">No inventory rows found.</td></tr>
                                    ) : groupedProducts.map((group) => {
                                        const expanded = expandedProductIDs.includes(group.productID);
                                        return (
                                            <Fragment key={group.productID}>
                                                <tr className="border-t border-[var(--border)]">
                                                    <td className="py-3">
                                                        <button
                                                            type="button"
                                                            className="inline-flex h-7 w-7 items-center justify-center rounded-md border border-[var(--border)] hover:bg-[var(--muted)]"
                                                            onClick={() =>
                                                                setExpandedProductIDs((prev) =>
                                                                    prev.includes(group.productID) ? prev.filter((id) => id !== group.productID) : [...prev, group.productID],
                                                                )
                                                            }
                                                            aria-label={expanded ? "Collapse variants" : "Expand variants"}
                                                        >
                                                            {expanded ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
                                                        </button>
                                                    </td>
                                                    <td className="py-3">
                                                        <div className="flex items-center gap-2">
                                                            {productThumbByID.get(group.productID) ? (
                                                                // eslint-disable-next-line @next/next/no-img-element
                                                                <img src={productThumbByID.get(group.productID) || ""} alt={group.productName} className="h-9 w-9 rounded-md object-cover" />
                                                            ) : (
                                                                <div className="h-9 w-9 rounded-md bg-[var(--muted)]" />
                                                            )}
                                                            <Link href={`/products/${group.productID}/edit`} className="hover:underline">
                                                                {group.productName}
                                                            </Link>
                                                        </div>
                                                    </td>
                                                    <td className="py-3">{group.variantsCount}</td>
                                                    <td className="py-3">{group.warehousesCount}</td>
                                                    <td className="py-3">
                                                        <span className={group.totalAvailable <= 5 ? "rounded-full bg-amber-100 px-2 py-0.5 text-amber-700" : ""}>
                                                            {group.totalAvailable}
                                                        </span>
                                                    </td>
                                                    <td className="py-3 text-xs text-[var(--muted-foreground)]">Expand to manage variants</td>
                                                </tr>
                                                {expanded ? (
                                                    <tr className="border-t border-[var(--border)] bg-[var(--muted)]/20">
                                                        <td colSpan={6} className="p-3">
                                                            <table className="min-w-full text-sm">
                                                                <thead className="text-left text-xs text-[var(--muted-foreground)]">
                                                                    <tr>
                                                                        <th className="pb-2">Variant</th>
                                                                        <th className="pb-2">SKU</th>
                                                                        <th className="pb-2">Warehouse</th>
                                                                        <th className="pb-2">Available</th>
                                                                        <th className="pb-2">Reserved</th>
                                                                        <th className="pb-2">Incoming</th>
                                                                        <th className="pb-2">Actions</th>
                                                                    </tr>
                                                                </thead>
                                                                <tbody>
                                                                    {group.variantRows.map((row) => (
                                                                        <tr key={row.variant_id} className="border-t border-[var(--border)]">
                                                                            <td className="py-2">{row.variant_name || "-"}</td>
                                                                            <td className="py-2">{row.sku || "-"}</td>
                                                                            <td className="py-2">
                                                                                {row.warehouses.length === 1 ? (
                                                                                    <button
                                                                                        type="button"
                                                                                        onClick={() => setWarehouseId(row.warehouses[0].id)}
                                                                                        className="hover:underline"
                                                                                    >
                                                                                        {row.warehouses[0].name}
                                                                                    </button>
                                                                                ) : (
                                                                                    <span className="text-xs text-[var(--muted-foreground)]">
                                                                                        Across {row.warehouses.length} locations
                                                                                    </span>
                                                                                )}
                                                                            </td>
                                                                            <td className="py-2">
                                                                                <span className={row.available_quantity <= 5 ? "rounded-full bg-amber-100 px-2 py-0.5 text-amber-700" : ""}>
                                                                                    {row.available_quantity}
                                                                                </span>
                                                                            </td>
                                                                            <td className="py-2">{row.reserved_quantity}</td>
                                                                            <td className="py-2 text-slate-400">{row.incoming_quantity}</td>
                                                                            <td className="py-2">
                                                                                <div className="flex gap-2">
                                                                                    <Button size="sm" variant="outline" onClick={() => { setSelectedVariant(row.actionRow); setAdjustOpen(true); }}>Edit</Button>
                                                                                    <Button size="sm" variant="outline" onClick={() => { setSelectedVariant(row.actionRow); setTransferOpen(true); }}>
                                                                                        <ArrowRightLeft className="size-3.5" /> Transfer
                                                                                    </Button>
                                                                                </div>
                                                                            </td>
                                                                        </tr>
                                                                    ))}
                                                                </tbody>
                                                            </table>
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
                </>
            )}

            {adjustOpen && selectedVariant && (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
                    <Card className="w-full max-w-md">
                        <CardContent className="space-y-3 pt-5">
                            <Label>Adjustment Type</Label>
                            <select value={adjustType} onChange={(e) => setAdjustType(e.target.value as "increase" | "decrease")} className="h-10 w-full rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3 text-sm">
                                <option value="increase">Increase (+)</option>
                                <option value="decrease">Decrease (-)</option>
                            </select>
                            <Label>Quantity</Label>
                            <Input type="number" min="1" value={adjustQty} onChange={(e) => setAdjustQty(e.target.value)} />
                            <Label>Reason (optional)</Label>
                            <Input value={adjustReason} onChange={(e) => setAdjustReason(e.target.value)} />
                            <div className="flex justify-end gap-2 pt-2">
                                <Button variant="outline" onClick={() => setAdjustOpen(false)}>Cancel</Button>
                                <Button loading={adjustMutation.isPending} onClick={() => adjustMutation.mutate()}>Apply</Button>
                            </div>
                        </CardContent>
                    </Card>
                </div>
            )}

            {transferOpen && selectedVariant && (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40">
                    <Card className="w-full max-w-md">
                        <CardContent className="space-y-3 pt-5">
                            <Label>From Warehouse</Label>
                            <Input value={warehouseChoices.find((w) => w.id === selectedVariant.warehouse_id)?.name || ""} disabled />
                            <Label>To Warehouse</Label>
                            <select value={transferTo} onChange={(e) => setTransferTo(e.target.value)} className="h-10 w-full rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3 text-sm">
                                <option value="">Select destination</option>
                                {warehouseChoices.filter((w) => w.id !== selectedVariant.warehouse_id).map((w) => (
                                    <option key={w.id} value={w.id}>{w.name}</option>
                                ))}
                            </select>
                            <Label>Quantity</Label>
                            <Input type="number" min="1" value={transferQty} onChange={(e) => setTransferQty(e.target.value)} />
                            <div className="flex justify-end gap-2 pt-2">
                                <Button variant="outline" onClick={() => setTransferOpen(false)}>Cancel</Button>
                                <Button loading={transferMutation.isPending} onClick={() => transferMutation.mutate()}>Transfer</Button>
                            </div>
                        </CardContent>
                    </Card>
                </div>
            )}
            {createWarehouseOpen && (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
                    <Card className="w-full max-w-lg">
                        <CardContent className="space-y-3 pt-5">
                            <h3 className="text-lg font-semibold">Create Warehouse</h3>
                            <div className="grid gap-3 md:grid-cols-2">
                                <div className="space-y-1.5 md:col-span-2">
                                    <Label>Name</Label>
                                    <Input value={warehouseName} onChange={(e) => setWarehouseName(e.target.value)} placeholder="Main Warehouse" />
                                </div>
                                <div className="space-y-1.5">
                                    <Label>Type</Label>
                                    <select
                                        value={warehouseType}
                                        onChange={(e) => setWarehouseType(e.target.value)}
                                        className="h-10 w-full rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3 text-sm"
                                    >
                                        <option value="warehouse">Warehouse</option>
                                        <option value="store">Store</option>
                                        <option value="dropship">Dropship</option>
                                        <option value="virtual">Virtual</option>
                                    </select>
                                </div>
                                <div className="space-y-1.5">
                                    <Label>Priority</Label>
                                    <Input type="number" min="1" value={warehousePriority} onChange={(e) => setWarehousePriority(e.target.value)} />
                                </div>
                                <div className="space-y-1.5">
                                    <Label>City</Label>
                                    <Input value={warehouseCity} onChange={(e) => setWarehouseCity(e.target.value)} />
                                </div>
                                <div className="space-y-1.5">
                                    <Label>Address</Label>
                                    <Input value={warehouseAddress} onChange={(e) => setWarehouseAddress(e.target.value)} />
                                </div>
                            </div>
                            <div className="flex justify-end gap-2 pt-2">
                                <Button variant="outline" onClick={() => setCreateWarehouseOpen(false)}>Cancel</Button>
                                <Button
                                    loading={createWarehouseMutation.isPending}
                                    disabled={!warehouseName.trim()}
                                    onClick={() => createWarehouseMutation.mutate()}
                                >
                                    Create
                                </Button>
                            </div>
                        </CardContent>
                    </Card>
                </div>
            )}
        </div>
    );
}
