"use client";

import { useParams } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowLeft, ArrowRightLeft, ImagePlus, Save } from "lucide-react";
import Link from "next/link";
import Barcode from "react-barcode";

import { Button, Card, CardContent, CardHeader, CardTitle, Input, Label } from "@/components/ui/primitives";
import { RichTextEditor } from "@/components/products/rich-text-editor";
import { SlugInput } from "@/components/products/slug-input";
import { MediaLibraryModal } from "@/components/media/media-library-modal";
import { SortableMediaGrid } from "@/components/media/sortable-media-grid";
import { VariantBuilder } from "@/components/products/variant-builder";
import { SeoSection } from "@/components/products/seo-section";
import { ProductStatusPanel } from "@/components/products/product-status-panel";
import { CategorySelector } from "@/components/products/category-selector";
import { CollectionMultiSelect } from "@/components/products/collection-multi-select";
import { api } from "@/lib/api-client";
import type { InventoryListItem, MediaAsset, ProductCreatePayload, ProductOption, ProductStatus, ProductVariantDraft } from "@/types/api";

// SKU generator
function generateSku() {
    const digits = Math.floor(10000000 + Math.random() * 90000000);
    return `JS-${digits}`;
}

export default function EditProductPage() {
    const params = useParams<{ id: string }>();
    const queryClient = useQueryClient();

    const { data: existing, isLoading } = useQuery({
        queryKey: ["product", params.id],
        queryFn: () => api.getProduct(params.id),
    });

    const [title, setTitle] = useState("");
    const [slug, setSlug] = useState("");
    const [description, setDescription] = useState("");
    const [categoryId, setCategoryId] = useState<string | null>(null);
    const [categoryName, setCategoryName] = useState("");
    const [collectionIds, setCollectionIds] = useState<string[]>([]);
    const [price, setPrice] = useState("");
    const [salePrice, setSalePrice] = useState("");
    const [cost, setCost] = useState("");
    const [chargeTax, setChargeTax] = useState(true);
    const [sku, setSku] = useState("");
    const [quantity, setQuantity] = useState("0");
    const [weight, setWeight] = useState("");
    const [options, setOptions] = useState<ProductOption[]>([]);
    const [variants, setVariants] = useState<ProductVariantDraft[]>([]);
    const [seoTitle, setSeoTitle] = useState("");
    const [seoDescription, setSeoDescription] = useState("");
    const [status, setStatus] = useState<ProductStatus>("draft");
    const [tags, setTags] = useState<string[]>([]);
    const [mediaOpen, setMediaOpen] = useState(false);
    const [manageInventoryOpen, setManageInventoryOpen] = useState(false);
    const [selectedInventoryRow, setSelectedInventoryRow] = useState<InventoryListItem | null>(null);
    const [adjustQty, setAdjustQty] = useState("0");
    const [adjustType, setAdjustType] = useState<"increase" | "decrease">("increase");
    const [adjustReason, setAdjustReason] = useState("");
    const [transferTo, setTransferTo] = useState("");
    const [transferQty, setTransferQty] = useState("0");
    const [hydratedSnapshotKey, setHydratedSnapshotKey] = useState<string | null>(null);
    const [isSaving, setIsSaving] = useState(false);
    const optionOrderStorageKey = `product-option-order:${params.id}`;
    const variantMediaStorageKey = `product-variant-media:${params.id}`;

    const heroMedia = useMemo(() => variants[0]?.media ?? [], [variants]);
    const setHeroMedia = useCallback((items: MediaAsset[]) => {
        setVariants((prev) => {
            if (!prev.length) return prev;
            return [{ ...prev[0], media: items }, ...prev.slice(1)];
        });
    }, []);

    // Populate from fetched product
    useEffect(() => {
        if (existing) {
            const productUpdatedAt = (existing.product?.updated_at as string | undefined) ?? "";
            const variantIds = ((existing.variants ?? []) as Record<string, unknown>[]).map((v) => String(v.id)).join(",");
            const collectionKey = [...(existing.collection_ids ?? [])].map(String).sort().join(",");
            const snapshotKey = `${params.id}:${productUpdatedAt}:${variantIds}:${collectionKey}`;
            if (snapshotKey === hydratedSnapshotKey) return;
            /* eslint-disable react-hooks/set-state-in-effect */
            setHydratedSnapshotKey(snapshotKey);
            const p = existing.product as typeof existing.product & { title?: string; slug?: string; status?: ProductStatus; tags?: string[]; seo_title?: string; seo_description?: string; sku?: string };
            setTitle(p.title ?? p.name ?? "");
            setSlug(p.slug ?? "");
            setDescription(p.description ?? "");
            setStatus(p.status ?? "draft");
            setTags(p.tags ?? []);
            setSeoTitle(p.seo_title ?? "");
            setSeoDescription(p.seo_description ?? "");
            setCollectionIds((existing.collection_ids ?? []).map(String));

            // Assume the API might return SKU natively, or fetch from first variant
            const rawVariants = (existing.variants as Array<Record<string, unknown>>) || [];
            const existingSku = p.sku ?? (rawVariants[0]?.sku as string | undefined);
            if (existingSku) setSku(existingSku);
            const storedVariantMediaBySku = (() => {
                try {
                    if (typeof window === "undefined") return {} as Record<string, MediaAsset[]>;
                    const raw = window.localStorage.getItem(variantMediaStorageKey);
                    if (!raw) return {} as Record<string, MediaAsset[]>;
                    const parsed = JSON.parse(raw) as Record<string, MediaAsset[]>;
                    return parsed && typeof parsed === "object" ? parsed : {};
                } catch {
                    return {} as Record<string, MediaAsset[]>;
                }
            })();

            const hydratedVariants: ProductVariantDraft[] = rawVariants.map((v) => {
                const skuKey = String(v.sku || "").trim().toUpperCase();
                const persistedMedia = Array.isArray(storedVariantMediaBySku[skuKey]) ? storedVariantMediaBySku[skuKey] : [];
                const backendMediaUrls = [
                    ...(Array.isArray(v.media_urls) ? v.media_urls : []),
                    ...(v.image_url ? [String(v.image_url)] : []),
                ].filter((url, idx, arr) => typeof url === "string" && url && arr.indexOf(url) === idx);
                const backendMedia = backendMediaUrls.map((url, idx) => ({
                    id: `variant-image:${String(v.id || "")}:${String(url)}`,
                    url: String(url),
                    mime_type: "image/*",
                    sort_order: idx,
                } as MediaAsset));
                const mergedMedia = [
                    ...backendMedia,
                    ...persistedMedia.filter(
                        (m) => m?.url && !backendMedia.some((b) => b.url === m.url),
                    ),
                ].filter((m, idx, arr) => m?.url && arr.findIndex((x) => x.url === m.url) === idx);
                const rawCost = v.cost ?? (v as Record<string, unknown>).unit_cost;
                let costStr: string | undefined;
                if (rawCost != null && String(rawCost).trim() !== "") {
                    const n = parseFloat(String(rawCost));
                    if (!isNaN(n)) costStr = n.toFixed(2);
                }
                return {
                id: String(v.id || ""),
                sku: String(v.sku || generateSku()),
                barcode: v.barcode ? String(v.barcode) : undefined,
                price: !isNaN(parseFloat(String(v.price))) ? parseFloat(String(v.price)).toFixed(2) : "0.00",
                sale_price: v.sale_price && !isNaN(parseFloat(String(v.sale_price))) ? parseFloat(String(v.sale_price)).toFixed(2) : undefined,
                cost: costStr,
                weight_g: typeof v.weight_g === "number" ? v.weight_g : undefined,
                quantity: typeof v.quantity === "number" ? v.quantity : 0,
                options: {
                    ...(v.size ? { Size: String(v.size) } : {}),
                    ...(v.color ? { Color: String(v.color) } : {}),
                    ...((v.options as Record<string, string> | undefined) || {}),
                },
                media: mergedMedia,
            };
            });
            setVariants(hydratedVariants);

            // Build options list from hydrated variant options
            const optionMap = new Map<string, Set<string>>();
            hydratedVariants.forEach((variant) => {
                Object.entries(variant.options || {}).forEach(([name, value]) => {
                    if (!name || !value) return;
                    if (!optionMap.has(name)) optionMap.set(name, new Set<string>());
                    optionMap.get(name)!.add(String(value));
                });
            });
            let hydratedOptions: ProductOption[] = Array.from(optionMap.entries()).map(([name, values]) => ({
                name,
                values: Array.from(values),
            }));
            try {
                const rawOrder = typeof window !== "undefined" ? window.localStorage.getItem(optionOrderStorageKey) : null;
                const preferredOrder = rawOrder ? (JSON.parse(rawOrder) as string[]) : [];
                if (Array.isArray(preferredOrder) && preferredOrder.length > 0) {
                    const indexByName = new Map(preferredOrder.map((n, i) => [n, i]));
                    hydratedOptions = [...hydratedOptions].sort((a, b) => {
                        const ai = indexByName.has(a.name) ? (indexByName.get(a.name) as number) : Number.MAX_SAFE_INTEGER;
                        const bi = indexByName.has(b.name) ? (indexByName.get(b.name) as number) : Number.MAX_SAFE_INTEGER;
                        return ai - bi;
                    });
                }
            } catch {
                // ignore invalid local order payload
            }
            setOptions(hydratedOptions);

            // Hydrate top-level pricing/quantity from first variant for convenience
            if (hydratedVariants.length > 0) {
                setPrice(hydratedVariants[0].price || "");
                setSalePrice(hydratedVariants[0].sale_price || "");
                setCost(hydratedVariants[0].cost ?? "");
                setQuantity(String(hydratedVariants[0].quantity ?? 0));
            } else {
                setPrice("");
                setSalePrice("");
                setCost("");
                setQuantity("0");
            }
            /* eslint-enable react-hooks/set-state-in-effect */
        }
    }, [existing, hydratedSnapshotKey, params.id]);

    const mutation = useMutation({
        mutationFn: (payload: Partial<ProductCreatePayload>) =>
            api.updateProduct(params.id, payload),
    });
    const { data: warehouses = [] } = useQuery({
        queryKey: ["warehouses"],
        queryFn: () => api.listWarehouses(),
    });
    const defaultWarehouse = warehouses[0];
    const { data: inventoryRowsResp } = useQuery({
        queryKey: ["inventory-rows-product-edit"],
        queryFn: () => api.listInventoryRows(),
    });
    const inventoryRows = useMemo(
        () => (inventoryRowsResp?.items ?? []).filter((row) => row.product_id === params.id),
        [inventoryRowsResp?.items, params.id],
    );
    const totalAvailable = inventoryRows.reduce((acc, row) => acc + row.available_quantity, 0);
    const locationsCount = new Set(inventoryRows.map((row) => row.warehouse_id)).size;
    const adjustInventory = useMutation({
        mutationFn: (): Promise<void> => {
            if (!selectedInventoryRow) return Promise.resolve();
            return api.adjustInventory({
                warehouse_id: selectedInventoryRow.warehouse_id,
                variant_id: selectedInventoryRow.variant_id,
                adjustment_type: adjustType,
                quantity: Math.max(0, Number(adjustQty || 0)),
                reason: adjustReason || undefined,
            }).then(() => undefined);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["inventory-rows-product-edit"] });
            setSelectedInventoryRow(null);
        },
    });
    const transferInventory = useMutation({
        mutationFn: (): Promise<void> => {
            if (!selectedInventoryRow || !transferTo) return Promise.resolve();
            return api.transferWarehouseStock({
                from_warehouse_id: selectedInventoryRow.warehouse_id,
                to_warehouse_id: transferTo,
                variant_id: selectedInventoryRow.variant_id,
                quantity: Math.max(0, Number(transferQty || 0)),
            }).then(() => undefined);
        },
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["inventory-rows-product-edit"] });
            setSelectedInventoryRow(null);
        },
    });

    const handleSave = useCallback(async () => {
        if (!title.trim()) { toast.error("Product title is required"); return; }
        try {
            setIsSaving(true);
            await mutation.mutateAsync({
                title: title.trim(),
                slug,
                description,
                status,
                category_id: categoryId,
                category: categoryName || undefined,
                tags,
                seo_title: seoTitle,
                seo_description: seoDescription,
                weight_g: weight ? Math.round(parseFloat(weight) * 1000) : undefined,
                collection_ids: collectionIds,
            });

            const existingIds = new Set(
                ((existing?.variants ?? []) as Record<string, unknown>[])
                    .map((v) => String(v.id || ""))
                    .filter(Boolean),
            );
            const existingIdBySku = new Map(
                ((existing?.variants ?? []) as Record<string, unknown>[])
                    .map((v) => [String(v.sku || "").trim().toUpperCase(), String(v.id || "")] as const)
                    .filter(([sku, id]) => Boolean(sku) && Boolean(id)),
            );

            for (const variant of variants) {
                const normalizedSku = (variant.sku || generateSku()).trim().toUpperCase();
                const firstUrl = variant.media?.[0]?.url;
                const payload = {
                    sku: normalizedSku,
                    color: variant.options?.Color || undefined,
                    size: variant.options?.Size || undefined,
                    image_url: firstUrl != null && firstUrl !== "" ? firstUrl : null,
                    media_urls: (variant.media ?? []).map((m) => m.url).filter(Boolean),
                    price: variant.price || undefined,
                    sale_price: variant.sale_price || undefined,
                    cost: (() => {
                        const row = String(variant.cost ?? "").trim();
                        if (row !== "") return row;
                        const top = String(cost ?? "").trim();
                        return top !== "" ? top : undefined;
                    })(),
                    quantity: typeof variant.quantity === "number" ? variant.quantity : 0,
                };
                if (!payload.sku) continue;

                const matchedExistingId =
                    (variant.id && existingIds.has(variant.id) ? variant.id : undefined) ??
                    existingIdBySku.get(normalizedSku);

                if (matchedExistingId) {
                    await api.patchVariant(matchedExistingId, payload);
                } else {
                    await api.createVariant(params.id, payload);
                }
            }

            if (typeof window !== "undefined") {
                const mediaBySku = Object.fromEntries(
                    variants
                        .map((v) => [String(v.sku || "").trim().toUpperCase(), (v.media || []).filter((m) => !!m?.url)] as const)
                        .filter(([sku]) => Boolean(sku)),
                );
                window.localStorage.setItem(variantMediaStorageKey, JSON.stringify(mediaBySku));
            }

            if (typeof window !== "undefined") {
                window.localStorage.setItem(optionOrderStorageKey, JSON.stringify(options.map((o) => o.name).filter(Boolean)));
            }

            queryClient.invalidateQueries({ queryKey: ["products"] });
            queryClient.invalidateQueries({ queryKey: ["collections"] });
            queryClient.invalidateQueries({ queryKey: ["product", params.id] });
            await queryClient.refetchQueries({ queryKey: ["product", params.id] });
            toast.success("Product updated");
        } catch (err) {
            toast.error(err instanceof Error ? err.message : "Failed to update product");
        } finally {
            setIsSaving(false);
        }
    }, [title, slug, description, status, categoryId, categoryName, collectionIds, tags, seoTitle, seoDescription, weight, mutation, existing?.variants, variants, cost, params.id, queryClient, optionOrderStorageKey, options, variantMediaStorageKey]);

    useEffect(() => {
        const handler = (e: KeyboardEvent) => {
            if ((e.metaKey || e.ctrlKey) && e.key === "s") { e.preventDefault(); handleSave(); }
        };
        window.addEventListener("keydown", handler);
        return () => window.removeEventListener("keydown", handler);
    }, [handleSave]);

    if (isLoading) {
        return (
            <div className="space-y-4">
                {[...Array(5)].map((_, i) => (
                    <div key={i} className="h-20 animate-pulse rounded-xl bg-[var(--muted)]" />
                ))}
            </div>
        );
    }

    return (
        <div className="space-y-6">
            <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                    <Button variant="ghost" size="icon" asChild>
                        <Link href="/products"><ArrowLeft className="size-4" /></Link>
                    </Button>
                    <div>
                        <h1 className="text-xl font-semibold">{title || "Edit product"}</h1>
                        <p className="text-xs text-[var(--muted-foreground)]">{status}</p>
                    </div>
                </div>
                <Button loading={mutation.isPending || isSaving} onClick={handleSave}>
                    <Save className="size-4" /> Save changes
                </Button>
            </div>

            <div className="grid gap-6 xl:grid-cols-[1fr_320px]">
                <div className="space-y-5">
                    <Card>
                        <CardContent className="space-y-4 pt-5">
                            <div className="space-y-1.5">
                                <Label>Title</Label>
                                <Input value={title} onChange={(e) => setTitle(e.target.value)} />
                            </div>
                            <SlugInput title={title} value={slug} onChange={setSlug} />
                        </CardContent>
                    </Card>

                    <Card>
                        <CardHeader><CardTitle>Description</CardTitle></CardHeader>
                        <CardContent>
                            <RichTextEditor value={description} onChange={setDescription} />
                        </CardContent>
                    </Card>

                    {/* Media */}
                    <Card>
                        <CardHeader><CardTitle>Media</CardTitle></CardHeader>
                        <CardContent>
                            <SortableMediaGrid items={heroMedia} onChange={setHeroMedia} />
                            <Button variant="outline" type="button" onClick={() => setMediaOpen(true)}>
                                <ImagePlus className="size-4" />
                                {heroMedia.length === 0 ? "Add media" : "Manage media"}
                            </Button>
                        </CardContent>
                    </Card>

                    <Card>
                        <CardHeader><CardTitle>Pricing</CardTitle></CardHeader>
                        <CardContent className="space-y-4">
                            <div className="grid grid-cols-2 gap-3">
                                <div className="space-y-1.5">
                                    <Label>Regular price (AED)</Label>
                                    <Input type="number" min="0" step="0.01" placeholder="0.00" value={price} onChange={(e) => setPrice(e.target.value)} />
                                </div>
                                <div className="space-y-1.5">
                                    <Label>Sale price (AED)</Label>
                                    <Input type="number" min="0" step="0.01" placeholder="0.00" value={salePrice} onChange={(e) => setSalePrice(e.target.value)} />
                                </div>
                            </div>
                            <div className="space-y-1.5">
                                <Label>Cost per item (AED)</Label>
                                <Input type="number" min="0" step="0.01" placeholder="0.00" value={cost} onChange={(e) => {
                                    const v = e.target.value;
                                    setCost(v);
                                    setVariants((prev) => prev.map((row) => ({ ...row, cost: v })));
                                }} />
                            </div>
                            <label className="flex cursor-pointer items-center gap-2 text-sm">
                                <input type="checkbox" checked={chargeTax} onChange={(e) => setChargeTax(e.target.checked)} className="accent-[var(--primary)]" />
                                Charge tax (5% UAE VAT)
                            </label>
                        </CardContent>
                    </Card>

                    {variants.length === 0 && (
                        <Card>
                            <CardHeader><CardTitle>Inventory</CardTitle></CardHeader>
                            <CardContent className="space-y-4">
                                <div className="grid grid-cols-2 gap-3">
                                    <div className="space-y-1.5">
                                        <Label>SKU</Label>
                                        <Input placeholder="JS-XXXXXXXX" value={sku} onChange={(e) => setSku(e.target.value)} />
                                    </div>
                                    <div className="space-y-1.5 flex flex-col justify-end">
                                        <Label className="mb-2">Barcode (<span className="text-[var(--muted-foreground)]">Scannable SKU</span>)</Label>
                                        <div className="h-10 flex items-center overflow-hidden">
                                            {sku ? (
                                                <Barcode value={sku} displayValue={false} height={30} width={1.5} margin={0} background="transparent" />
                                            ) : (
                                                <span className="text-xs text-[var(--muted-foreground)] text-center w-full">Enter SKU to generate barcode</span>
                                            )}
                                        </div>
                                    </div>
                                </div>
                                <div className="space-y-1.5">
                                    <Label>Quantity</Label>
                                    <Input type="number" min="0" value={quantity} onChange={(e) => setQuantity(e.target.value)} />
                                </div>
                            </CardContent>
                        </Card>
                    )}

                    <Card>
                        <CardHeader><CardTitle>Shipping</CardTitle></CardHeader>
                        <CardContent>
                            <div className="flex items-center gap-2">
                                <div className="flex-1 space-y-1.5">
                                    <Label>Weight</Label>
                                    <Input type="number" min="0" step="0.01" placeholder="0.00" value={weight} onChange={(e) => setWeight(e.target.value)} />
                                </div>
                                <div className="space-y-1.5">
                                    <Label>Unit</Label>
                                    <div className="flex h-10 items-center rounded-xl border border-[var(--border)] bg-[var(--muted)] px-3 text-sm">kg</div>
                                </div>
                            </div>
                        </CardContent>
                    </Card>

                    <VariantBuilder options={options} onOptionsChange={setOptions} variants={variants} onVariantsChange={setVariants} nextSku={generateSku} basePrice={price} baseCost={cost} />

                    <SeoSection title={seoTitle} description={seoDescription} urlHandle={slug} onTitleChange={setSeoTitle} onDescriptionChange={setSeoDescription} />
                </div>

                <div className="space-y-4">
                    <Card>
                        <CardHeader><CardTitle>Inventory</CardTitle></CardHeader>
                        <CardContent className="space-y-3 text-sm">
                            <div className="flex items-center justify-between">
                                <span className="text-[var(--muted-foreground)]">Total Available</span>
                                <span className="font-semibold">{totalAvailable}</span>
                            </div>
                            <div className="flex items-center justify-between">
                                <span className="text-[var(--muted-foreground)]">Locations</span>
                                <span className="font-semibold">{locationsCount || 0} warehouses</span>
                            </div>
                            <Button variant="outline" className="w-full" onClick={() => setManageInventoryOpen(true)}>
                                Manage Inventory
                            </Button>
                        </CardContent>
                    </Card>
                    <Card>
                        <CardHeader><CardTitle>Category</CardTitle></CardHeader>
                        <CardContent>
                            <CategorySelector
                                value={categoryId}
                                onChange={(id, category) => {
                                    setCategoryId(id);
                                    setCategoryName(category?.title || "");
                                }}
                            />
                        </CardContent>
                    </Card>
                    <Card>
                        <CardHeader><CardTitle>Collections</CardTitle></CardHeader>
                        <CardContent className="space-y-2">
                            <p className="text-xs text-[var(--muted-foreground)]">
                                Merchandising groups (e.g. promos, seasonal edits). Separate from structural categories above.
                            </p>
                            <CollectionMultiSelect value={collectionIds} onChange={setCollectionIds} />
                        </CardContent>
                    </Card>
                    <ProductStatusPanel status={status} onStatusChange={setStatus} tags={tags} onTagsChange={setTags} />
                </div>
            </div>

            <MediaLibraryModal open={mediaOpen} onOpenChange={setMediaOpen} value={heroMedia} onChange={setHeroMedia} />
            {manageInventoryOpen && (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
                    <Card className="w-full max-w-4xl">
                        <CardContent className="space-y-4 p-5">
                            <div className="flex items-center justify-between">
                                <h3 className="text-lg font-semibold">Manage Inventory</h3>
                                <Button variant="outline" onClick={() => setManageInventoryOpen(false)}>Close</Button>
                            </div>
                            <div className="overflow-x-auto">
                                <table className="min-w-full text-sm">
                                    <thead>
                                        <tr className="text-left text-xs text-[var(--muted-foreground)]">
                                            <th className="pb-2">Warehouse</th>
                                            <th className="pb-2">Available</th>
                                            <th className="pb-2">Reserved</th>
                                            <th className="pb-2">Incoming</th>
                                            <th className="pb-2">Actions</th>
                                        </tr>
                                    </thead>
                                    <tbody>
                                        {inventoryRows.map((row) => (
                                            <tr key={`${row.warehouse_id}-${row.variant_id}`} className={`border-t border-[var(--border)] ${defaultWarehouse?.id === row.warehouse_id ? "bg-emerald-50/40" : ""}`}>
                                                <td className="py-2">{row.warehouse}{defaultWarehouse?.id === row.warehouse_id ? " (Default)" : ""}</td>
                                                <td className="py-2">{row.available_quantity}</td>
                                                <td className="py-2">{row.reserved_quantity}</td>
                                                <td className="py-2 text-slate-400">{row.incoming_quantity}</td>
                                                <td className="py-2">
                                                    <div className="flex gap-2">
                                                        <Button size="sm" variant="outline" onClick={() => { setSelectedInventoryRow(row); setAdjustType("increase"); }}>
                                                            Edit
                                                        </Button>
                                                        <Button size="sm" variant="outline" onClick={() => { setSelectedInventoryRow(row); }}>
                                                            <ArrowRightLeft className="size-3.5" /> Transfer
                                                        </Button>
                                                    </div>
                                                </td>
                                            </tr>
                                        ))}
                                    </tbody>
                                </table>
                            </div>
                        </CardContent>
                    </Card>
                </div>
            )}
            {selectedInventoryRow && (
                <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/40 p-4">
                    <Card className="w-full max-w-md">
                        <CardContent className="space-y-3 p-5">
                            <Label>Adjustment type</Label>
                            <select value={adjustType} onChange={(e) => setAdjustType(e.target.value as "increase" | "decrease")} className="h-10 w-full rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3 text-sm">
                                <option value="increase">Increase (+)</option>
                                <option value="decrease">Decrease (-)</option>
                            </select>
                            <Label>Quantity</Label>
                            <Input type="number" min="1" value={adjustQty} onChange={(e) => setAdjustQty(e.target.value)} />
                            <Label>Reason (optional)</Label>
                            <Input value={adjustReason} onChange={(e) => setAdjustReason(e.target.value)} />
                            <Label>Transfer to (optional)</Label>
                            <select value={transferTo} onChange={(e) => setTransferTo(e.target.value)} className="h-10 w-full rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3 text-sm">
                                <option value="">No transfer</option>
                                {warehouses.filter((w) => w.id !== selectedInventoryRow.warehouse_id).map((w) => (
                                    <option key={w.id} value={w.id}>{w.name}</option>
                                ))}
                            </select>
                            <Label>Transfer quantity</Label>
                            <Input type="number" min="1" value={transferQty} onChange={(e) => setTransferQty(e.target.value)} />
                            <div className="flex justify-end gap-2 pt-2">
                                <Button variant="outline" onClick={() => setSelectedInventoryRow(null)}>Cancel</Button>
                                <Button loading={adjustInventory.isPending} onClick={() => adjustInventory.mutate()}>Apply Adjustment</Button>
                                <Button loading={transferInventory.isPending} disabled={!transferTo} onClick={() => transferInventory.mutate()}>Transfer</Button>
                            </div>
                        </CardContent>
                    </Card>
                </div>
            )}
        </div>
    );
}
