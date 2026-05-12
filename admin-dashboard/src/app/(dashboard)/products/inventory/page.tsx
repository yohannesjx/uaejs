"use client";

import { Fragment, useCallback, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BarChart2, ArrowRightLeft, ChevronDown, ChevronRight } from "lucide-react";
import { api, publicUploadUrl } from "@/lib/api-client";
import { PageHeader } from "@/components/dashboard-blocks";
import Link from "next/link";
import { Button, Card, CardContent, Input, Label } from "@/components/ui/primitives";

function formatAedAmount(n: number): string {
    if (!Number.isFinite(n)) return "0.00";
    return n.toLocaleString("en-AE", { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

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
    /** Variant IDs selected for aggregate totals (product checkbox selects all variants of that product). */
    const [selectedVariants, setSelectedVariants] = useState<Record<string, boolean>>({});

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
    const warehouseCostTotals = useMemo(() => {
        const m = new Map<string, { name: string; total: number }>();
        for (const r of rows) {
            const w = r.warehouse_id;
            const name = r.warehouse;
            const line = parseFloat(String(r.stock_value_at_cost || "0")) || 0;
            const prev = m.get(w) ?? { name, total: 0 };
            prev.total += line;
            m.set(w, prev);
        }
        return Array.from(m.entries()).map(([id, v]) => ({ id, name: v.name, total: v.total }));
    }, [rows]);
    const grandStockValueAtCost = useMemo(
        () => rows.reduce((acc, r) => acc + (parseFloat(String(r.stock_value_at_cost || "0")) || 0), 0),
        [rows],
    );
    const grandTotalQuantity = useMemo(
        () => rows.reduce((acc, r) => acc + (r.available_quantity || 0), 0),
        [rows],
    );
    const grandExpectedRevenue = useMemo(
        () => rows.reduce((acc, r) => acc + (parseFloat(String(r.stock_value_at_revenue || "0")) || 0), 0),
        [rows],
    );

    const selectedTotals = useMemo(() => {
        const seenVariant = new Set<string>();
        let qty = 0;
        let cost = 0;
        let revenue = 0;
        for (const r of rows) {
            if (!selectedVariants[r.variant_id]) continue;
            seenVariant.add(r.variant_id);
            qty += r.available_quantity || 0;
            cost += parseFloat(String(r.stock_value_at_cost || "0")) || 0;
            revenue += parseFloat(String(r.stock_value_at_revenue || "0")) || 0;
        }
        return {
            qty,
            cost,
            revenue,
            selectedVariantCount: seenVariant.size,
        };
    }, [rows, selectedVariants]);

    const toggleVariantSelected = useCallback((variantId: string, selected: boolean) => {
        setSelectedVariants((prev) => {
            const next = { ...prev };
            if (selected) next[variantId] = true;
            else delete next[variantId];
            return next;
        });
    }, []);

    const toggleProductVariantsSelected = useCallback((variantIds: string[], selectAll: boolean) => {
        setSelectedVariants((prev) => {
            const next = { ...prev };
            if (selectAll) {
                variantIds.forEach((id) => {
                    next[id] = true;
                });
            } else {
                variantIds.forEach((id) => {
                    delete next[id];
                });
            }
            return next;
        });
    }, []);
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
            const totalStockValueAtCost = productRows.reduce(
                (acc, curr) => acc + (parseFloat(String(curr.stock_value_at_cost || "0")) || 0),
                0,
            );
            const totalExpectedRevenue = productRows.reduce(
                (acc, curr) => acc + (parseFloat(String(curr.stock_value_at_revenue || "0")) || 0),
                0,
            );
            const variantsCount = new Set(productRows.map((x) => x.variant_id)).size;
            const warehousesCount = new Set(productRows.map((x) => x.warehouse_id)).size;
            return {
                productID,
                productName: first?.product_name || "Unknown product",
                variantsCount,
                warehousesCount,
                totalAvailable,
                totalStockValueAtCost,
                totalExpectedRevenue,
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
                            unit_cost: row.unit_cost as string | null | undefined,
                            line_stock_value: 0,
                            regular_price: row.regular_price as string | null | undefined,
                            line_expected_revenue: 0,
                        };
                        current.available_quantity += row.available_quantity;
                        current.reserved_quantity += row.reserved_quantity;
                        current.incoming_quantity += row.incoming_quantity;
                        current.line_stock_value += parseFloat(String(row.stock_value_at_cost || "0")) || 0;
                        current.line_expected_revenue += parseFloat(String(row.stock_value_at_revenue || "0")) || 0;
                        if (!current.unit_cost && row.unit_cost) current.unit_cost = row.unit_cost;
                        if (!current.regular_price && row.regular_price) current.regular_price = row.regular_price;
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
                        unit_cost?: string | null;
                        line_stock_value: number;
                        regular_price?: string | null;
                        line_expected_revenue: number;
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
                        <CardContent className="flex flex-col gap-5 pt-5">
                            <div className="grid gap-6 sm:grid-cols-3">
                                <div>
                                    <div className="text-xs font-semibold uppercase tracking-wider text-[var(--muted-foreground)]">Inventory value at unit cost</div>
                                    <div className="text-2xl font-semibold tabular-nums">{formatAedAmount(grandStockValueAtCost)} AED</div>
                                    <p className="text-xs text-[var(--muted-foreground)]">Sum of (available × unit cost) for rows in the table below.</p>
                                </div>
                                <div>
                                    <div className="text-xs font-semibold uppercase tracking-wider text-[var(--muted-foreground)]">Expected revenue</div>
                                    <div className="text-2xl font-semibold tabular-nums">{formatAedAmount(grandExpectedRevenue)} AED</div>
                                    <p className="text-xs text-[var(--muted-foreground)]">If every available unit sold at the regular list price (channel price).</p>
                                </div>
                                <div>
                                    <div className="text-xs font-semibold uppercase tracking-wider text-[var(--muted-foreground)]">Total quantity</div>
                                    <div className="text-2xl font-semibold tabular-nums">{grandTotalQuantity}</div>
                                    <p className="text-xs text-[var(--muted-foreground)]">Available units summed across all rows in the table.</p>
                                </div>
                            </div>
                            <div className="flex flex-wrap gap-x-6 gap-y-2 border-t border-[var(--border)] pt-4 text-sm">
                                <div className="text-xs font-semibold uppercase tracking-wider text-[var(--muted-foreground)] w-full">By warehouse (value at cost)</div>
                                {warehouseCostTotals.length === 0 ? (
                                    <span className="text-[var(--muted-foreground)]">No cost data (set unit cost on each variant in the product editor).</span>
                                ) : (
                                    warehouseCostTotals.map((w) => (
                                        <div key={w.id} className="min-w-[140px]">
                                            <div className="text-xs text-[var(--muted-foreground)]">{w.name}</div>
                                            <div className="font-medium tabular-nums">{formatAedAmount(w.total)} AED</div>
                                        </div>
                                    ))
                                )}
                            </div>
                        </CardContent>
                    </Card>
                    {selectedTotals.selectedVariantCount > 0 && (
                        <Card className="border-[var(--primary)]/30 bg-[var(--primary)]/5">
                            <CardContent className="flex flex-col gap-3 pt-5 sm:flex-row sm:flex-wrap sm:items-center sm:justify-between">
                                <div className="text-sm font-medium text-[var(--foreground)]">
                                    {selectedTotals.selectedVariantCount} variant{selectedTotals.selectedVariantCount === 1 ? "" : "s"} selected
                                </div>
                                <div className="flex flex-wrap gap-6 text-sm tabular-nums">
                                    <div>
                                        <div className="text-xs text-[var(--muted-foreground)]">Inventory cost</div>
                                        <div className="font-semibold">{formatAedAmount(selectedTotals.cost)} AED</div>
                                    </div>
                                    <div>
                                        <div className="text-xs text-[var(--muted-foreground)]">Expected sale</div>
                                        <div className="font-semibold">{formatAedAmount(selectedTotals.revenue)} AED</div>
                                    </div>
                                    <div>
                                        <div className="text-xs text-[var(--muted-foreground)]">Quantity</div>
                                        <div className="font-semibold">{selectedTotals.qty}</div>
                                    </div>
                                </div>
                                <Button type="button" variant="outline" size="sm" onClick={() => setSelectedVariants({})}>
                                    Clear selection
                                </Button>
                            </CardContent>
                        </Card>
                    )}
                    <Card>
                        <CardContent className="overflow-x-auto pt-5">
                            <table className="min-w-full text-sm">
                                <thead className="text-left text-xs text-[var(--muted-foreground)]">
                                    <tr>
                                        <th className="pb-2 w-10" aria-label="Select" />
                                        <th className="pb-2 w-10" aria-hidden />
                                        <th className="pb-2">Product</th>
                                        <th className="pb-2">Variants</th>
                                        <th className="pb-2">Locations</th>
                                        <th className="pb-2">Total Available</th>
                                        <th className="pb-2 text-right">Value at cost</th>
                                        <th className="pb-2 text-right">Expected revenue</th>
                                        <th className="pb-2">Actions</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {isLoading ? (
                                        <tr><td colSpan={9} className="py-6 text-center text-[var(--muted-foreground)]">Loading inventory...</td></tr>
                                    ) : groupedProducts.length === 0 ? (
                                        <tr><td colSpan={9} className="py-6 text-center text-[var(--muted-foreground)]">No inventory rows found.</td></tr>
                                    ) : groupedProducts.map((group) => {
                                        const expanded = expandedProductIDs.includes(group.productID);
                                        const variantIds = group.variantRows.map((v) => v.variant_id);
                                        const allVariantsSelected = variantIds.length > 0 && variantIds.every((id) => selectedVariants[id]);
                                        const someVariantsSelected = variantIds.some((id) => selectedVariants[id]);
                                        return (
                                            <Fragment key={group.productID}>
                                                <tr className="border-t border-[var(--border)]">
                                                    <td className="py-3 align-middle">
                                                        <input
                                                            type="checkbox"
                                                            className="size-4 rounded border-[var(--border)] accent-[var(--primary)]"
                                                            checked={allVariantsSelected}
                                                            ref={(el) => {
                                                                if (el) el.indeterminate = someVariantsSelected && !allVariantsSelected;
                                                            }}
                                                            onChange={() => toggleProductVariantsSelected(variantIds, !allVariantsSelected)}
                                                            aria-label={`Select all variants for ${group.productName}`}
                                                        />
                                                    </td>
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
                                                                <img src={publicUploadUrl(productThumbByID.get(group.productID) || "")} alt={group.productName} className="h-9 w-9 rounded-md object-cover" />
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
                                                    <td className="py-3 text-right tabular-nums text-[var(--muted-foreground)]">
                                                        {formatAedAmount(group.totalStockValueAtCost)} AED
                                                    </td>
                                                    <td className="py-3 text-right tabular-nums text-[var(--muted-foreground)]">
                                                        {formatAedAmount(group.totalExpectedRevenue)} AED
                                                    </td>
                                                    <td className="py-3 text-xs text-[var(--muted-foreground)]">Expand to manage variants</td>
                                                </tr>
                                                {expanded ? (
                                                    <tr className="border-t border-[var(--border)] bg-[var(--muted)]/20">
                                                        <td colSpan={9} className="p-3">
                                                            <table className="min-w-full text-sm">
                                                                <thead className="text-left text-xs text-[var(--muted-foreground)]">
                                                                    <tr>
                                                                        <th className="pb-2 w-10" aria-label="Select variant" />
                                                                        <th className="pb-2">Variant</th>
                                                                        <th className="pb-2">SKU</th>
                                                                        <th className="pb-2">Warehouse</th>
                                                                        <th className="pb-2">Available</th>
                                                                        <th className="pb-2">Reserved</th>
                                                                        <th className="pb-2">Incoming</th>
                                                                        <th className="pb-2 text-right">Unit cost</th>
                                                                        <th className="pb-2 text-right">Stock value</th>
                                                                        <th className="pb-2 text-right">List price</th>
                                                                        <th className="pb-2 text-right">Expected revenue</th>
                                                                        <th className="pb-2">Actions</th>
                                                                    </tr>
                                                                </thead>
                                                                <tbody>
                                                                    {group.variantRows.map((row) => (
                                                                        <tr key={row.variant_id} className="border-t border-[var(--border)]">
                                                                            <td className="py-2 align-middle">
                                                                                <input
                                                                                    type="checkbox"
                                                                                    className="size-4 rounded border-[var(--border)] accent-[var(--primary)]"
                                                                                    checked={!!selectedVariants[row.variant_id]}
                                                                                    onChange={() =>
                                                                                        toggleVariantSelected(row.variant_id, !selectedVariants[row.variant_id])
                                                                                    }
                                                                                    aria-label={`Select ${row.variant_name || row.sku}`}
                                                                                />
                                                                            </td>
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
                                                                            <td className="py-2 text-right tabular-nums text-xs text-[var(--muted-foreground)]">
                                                                                {row.unit_cost != null && String(row.unit_cost).trim() !== ""
                                                                                    ? `${formatAedAmount(parseFloat(String(row.unit_cost)) || 0)} AED`
                                                                                    : "—"}
                                                                            </td>
                                                                            <td className="py-2 text-right tabular-nums text-xs">
                                                                                {formatAedAmount(row.line_stock_value)} AED
                                                                            </td>
                                                                            <td className="py-2 text-right tabular-nums text-xs text-[var(--muted-foreground)]">
                                                                                {row.regular_price != null && String(row.regular_price).trim() !== ""
                                                                                    ? `${formatAedAmount(parseFloat(String(row.regular_price)) || 0)} AED`
                                                                                    : "—"}
                                                                            </td>
                                                                            <td className="py-2 text-right tabular-nums text-xs">
                                                                                {formatAedAmount(row.line_expected_revenue)} AED
                                                                            </td>
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
