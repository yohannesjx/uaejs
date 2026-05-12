"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";
import { useForm, useWatch, FormProvider } from "react-hook-form";
import { toast } from "sonner";
import { ArrowLeft, ImagePlus, Save, Loader2, CheckCircle2 } from "lucide-react";
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
import { ApiError, api } from "@/lib/api-client";
import type { MediaAsset, ProductOption, ProductStatus, ProductVariantDraft } from "@/types/api";

function generateSku() {
    const digits = Math.floor(10000000 + Math.random() * 90000000);
    return `JS-${digits}`;
}

type ProductFormValues = {
    title: string;
    slug: string;
    description: string;
    categoryId: string | null;
    trackInventory: boolean;
    inventoryWarehouseId: string | null;
    price: string;
    salePrice: string;
    cost: string;
    chargeTax: boolean;
    sku: string;
    quantity: string;
    weight: string;
    weightUnit: string;
    options: unknown[];
    variants: Array<Record<string, unknown>>;
    seoTitle: string;
    seoDescription: string;
    status: ProductStatus;
    tags: string[];
};

const initialFormValues = {
    title: "",
    slug: "",
    description: "",
    categoryId: null,
    trackInventory: true,
    inventoryWarehouseId: null,
    price: "",
    salePrice: "",
    cost: "",
    chargeTax: true,
    sku: generateSku(),
    quantity: "0",
    weight: "",
    weightUnit: "kg",
    options: [],
    variants: [],
    seoTitle: "",
    seoDescription: "",
    status: "active" as ProductStatus,
    tags: [],
};

export default function NewProductPage() {
    const router = useRouter();

    const [draftId, setDraftId] = useState<string | null>(null);
    const [saveStatus, setSaveStatus] = useState<"idle" | "saving" | "saved" | "error">("idle");
    const [mediaOpen, setMediaOpen] = useState(false);

    // State for things that are complex to hook into react-hook-form natively initially
    const [media, setMedia] = useState<MediaAsset[]>([]);
    const isFirstMount = useRef(true);

    const methods = useForm<ProductFormValues>({
        defaultValues: initialFormValues,
    });

    const { register, control, setValue, getValues, reset, formState: { isDirty } } = methods;
    const [mediaDirty, setMediaDirty] = useState(false);
    const [selectedCategoryName, setSelectedCategoryName] = useState<string>("");
    const { data: warehouses = [] } = useQuery({
        queryKey: ["warehouses"],
        queryFn: () => api.listWarehouses(),
    });
    const [leaveDialogOpen, setLeaveDialogOpen] = useState(false);
    const [showUnsavedLeaveBanner, setShowUnsavedLeaveBanner] = useState(false);
    const [pendingHref, setPendingHref] = useState<string | null>(null);
    const hasUnsavedChanges = isDirty || mediaDirty;

    // We use useWatch to subscribe to form variations without re-rendering the whole tree unconditionally
    const formVals = useWatch({ control });
    const prevPricingBase = useRef({ price: "", salePrice: "", cost: "" });

    /** When top-level price / sale / cost changes, apply to every variant (variants can still be edited per row). */
    useEffect(() => {
        const variants = formVals.variants || [];
        if (!Array.isArray(variants) || variants.length === 0) return;
        const price = formVals.price ?? "";
        const salePrice = formVals.salePrice ?? "";
        const cost = formVals.cost ?? "";
        const prev = prevPricingBase.current;
        const baseChanged = prev.price !== price || prev.salePrice !== salePrice || prev.cost !== cost;
        prevPricingBase.current = { price, salePrice, cost };
        if (!baseChanged) return;

        const next = variants.map((v: Record<string, unknown>) => ({
            ...v,
            price,
            sale_price: salePrice,
            cost,
        }));

        const changed = next.some((v: Record<string, unknown>, idx: number) => {
            const cur = variants[idx] as Record<string, unknown>;
            return v.price !== cur.price || v.sale_price !== cur.sale_price || v.cost !== cur.cost;
        });
        if (changed) {
            setValue("variants", next, { shouldDirty: true });
        }
    }, [formVals.price, formVals.salePrice, formVals.cost, formVals.variants, setValue]);

    const productMediaKey = useMemo(() => media.map((m, i) => `${i}:${m.id ?? ""}:${m.url}`).join("\0"), [media]);
    const variantsLen = Array.isArray(formVals.variants) ? formVals.variants.length : 0;

    /** When product-level media changes (or variant matrix appears), copy that gallery onto every variant for save/API parity with edit product. */
    useEffect(() => {
        const rows = getValues("variants") as Array<Record<string, unknown>>;
        if (!Array.isArray(rows) || rows.length === 0) return;
        const inherited = media.map((m, i) => ({ ...m, sort_order: i }));
        const next = rows.map((v) => ({ ...v, media: inherited }));
        const changed = next.some((v, idx) => {
            const cur = rows[idx];
            const a = (cur.media as MediaAsset[]) || [];
            const b = (v.media as MediaAsset[]) || [];
            if (a.length !== b.length) return true;
            return a.some((item, i) => item?.url !== b[i]?.url);
        });
        if (changed) setValue("variants", next, { shouldDirty: true });
    }, [productMediaKey, variantsLen, media, getValues, setValue]);

    // 1. Initialize empty draft on mount
    useEffect(() => {
        if (!isFirstMount.current) return;
        isFirstMount.current = false;

        api.createDraftProduct()
            .then((res) => setDraftId(res.id))
            .catch((err) => {
                toast.error("Failed to initialize product auto-save draft");
                console.error(err);
            });
    }, []);

    // 2. Auto-save Patch requests (debounced 800ms)
    useEffect(() => {
        if (!draftId || !isDirty) return;

        const handle = setTimeout(async () => {
            setSaveStatus("saving");
            try {
                // Map the form layout into the structured Patch Payload backing only core attributes currently mapped in the backend patch domain
                const patchPayload: Record<string, unknown> = {
                    name: formVals.title || undefined,
                    description: formVals.description || undefined,
                    status: "draft",
                    category_id: formVals.categoryId || undefined,
                    category: selectedCategoryName || undefined,
                    track_inventory: Boolean(formVals.trackInventory),
                    warehouse_id: formVals.inventoryWarehouseId || undefined,
                    // tags: formVals.tags,
                };

                await api.updateProduct(draftId, patchPayload);
                setSaveStatus("saved");
            } catch (err) {
                console.error("Auto-save failed", err);
                setSaveStatus("error");
            }
        }, 800);

        return () => clearTimeout(handle);
    }, [formVals, draftId, isDirty]);

    // 3. Beforeunload blocker
    useEffect(() => {
        const onBeforeUnload = (e: BeforeUnloadEvent) => {
            if (saveStatus === "saving" || hasUnsavedChanges) {
                e.preventDefault();
                e.returnValue = "";
            }
        };
        window.addEventListener("beforeunload", onBeforeUnload);
        return () => window.removeEventListener("beforeunload", onBeforeUnload);
    }, [saveStatus, hasUnsavedChanges]);

    useEffect(() => {
        if (warehouses.length === 0) return;
        if (getValues("inventoryWarehouseId")) return;
        setValue("inventoryWarehouseId", warehouses[0].id, { shouldDirty: false });
    }, [warehouses, getValues, setValue]);

    useEffect(() => {
        const onDocumentClick = (e: MouseEvent) => {
            if (!hasUnsavedChanges) return;
            const target = e.target as HTMLElement | null;
            if (!target) return;
            const anchor = target.closest("a[href]") as HTMLAnchorElement | null;
            if (!anchor) return;
            const href = anchor.getAttribute("href");
            if (!href || href.startsWith("#") || href.startsWith("mailto:") || href.startsWith("tel:")) return;
            if (href === window.location.pathname) return;
            if (!href.startsWith("/")) return;
            e.preventDefault();
            setPendingHref(href);
            setShowUnsavedLeaveBanner(true);
            setLeaveDialogOpen(true);
        };

        document.addEventListener("click", onDocumentClick, true);
        return () => document.removeEventListener("click", onDocumentClick, true);
    }, [hasUnsavedChanges]);

    // 4. Manual Publish Handlers
    const handleSave = useCallback(async (forcedStatus?: ProductStatus) => {
        const vals = getValues();
        if (!vals.title?.trim()) {
            toast.error("Product title is required");
            return;
        }

        const intQty = (primary: unknown, fallback: unknown): number => {
            const raw = primary != null && primary !== "" ? primary : fallback;
            if (typeof raw === "number" && Number.isFinite(raw)) return Math.max(0, Math.floor(raw));
            const n = Number.parseInt(String(raw ?? "0"), 10);
            return Number.isFinite(n) && n >= 0 ? n : 0;
        };

        setSaveStatus("saving");
        try {
            const fallbackMediaUrl = media[0]?.url;
            const inputVariants = Array.isArray(vals.variants) ? vals.variants : [];
            const seenSku = new Set<string>();
            const variants = inputVariants.map((v) => {
                const rec = v as Record<string, unknown>;
                const variantMedia = Array.isArray(rec.media) ? (rec.media as MediaAsset[]) : [];
                const mergedMedia = variantMedia.length > 0
                    ? variantMedia
                    : (fallbackMediaUrl ? [{ id: "fallback-media", url: fallbackMediaUrl, mime_type: "image/*", sort_order: 0 } as unknown as MediaAsset] : []);
                const skuPattern = /^JS-\d{8}$/;
                const preferredSku = String(rec.sku || vals.sku || "").trim().toUpperCase();
                let finalSku = skuPattern.test(preferredSku) ? preferredSku : generateSku().toUpperCase();
                while (seenSku.has(finalSku)) {
                    finalSku = generateSku().toUpperCase();
                }
                seenSku.add(finalSku);
                return {
                    sku: finalSku,
                    barcode: rec.barcode ? String(rec.barcode) : undefined,
                    price: String(rec.price || vals.price || "0"),
                    sale_price: rec.sale_price ? String(rec.sale_price) : (vals.salePrice || undefined),
                    cost: String(rec.cost || vals.cost || "0"),
                    weight_g: rec.weight_g ? Number(rec.weight_g) : undefined,
                    quantity: intQty(rec.quantity, vals.quantity),
                    options: (rec.options as Record<string, string>) || {},
                    media: mergedMedia,
                } as ProductVariantDraft;
            });

            if (variants.length === 0) {
                variants.push({
                    sku: String(vals.sku || generateSku()).toUpperCase(),
                    price: String(vals.price || "0"),
                    sale_price: vals.salePrice || undefined,
                    cost: String(vals.cost || "0"),
                    quantity: intQty(vals.quantity, 0),
                    options: {},
                    media: fallbackMediaUrl ? [{ id: "fallback-media", url: fallbackMediaUrl, mime_type: "image/*", sort_order: 0 } as unknown as MediaAsset] : [],
                });
            }

            const effectiveStatus = forcedStatus ?? vals.status ?? "active";

            await api.createProductV2({
                title: vals.title.trim(),
                slug: vals.slug?.trim() || vals.title.trim(),
                description: vals.description || "",
                status: effectiveStatus,
                category_id: vals.categoryId || undefined,
                category: selectedCategoryName || undefined,
                track_inventory: Boolean(vals.trackInventory),
                warehouse_id: vals.inventoryWarehouseId || undefined,
                tags: vals.tags || [],
                seo_title: vals.seoTitle || "",
                seo_description: vals.seoDescription || "",
                weight_g: vals.weight ? Math.round(parseFloat(vals.weight) * 1000) : undefined,
                options: (vals.options as { name: string; values: string[] }[]) || [],
                variants,
            });

            // Publish creates the canonical product record; remove bootstrap draft
            if (draftId) {
                try {
                    await api.deleteProduct(draftId);
                } catch {
                    // Non-fatal cleanup failure; published product already exists
                }
            }
            toast.success("Product Saved successfully!");
            setSaveStatus("saved");
            setMediaDirty(false);

            // Redirect or invalidate
            router.push(`/products`);
        } catch (err) {
            if (err instanceof ApiError && typeof err.payload === "string" && err.payload.trim()) {
                toast.error(err.payload);
            } else {
                toast.error("Failed to save complete product active state.");
            }
            setSaveStatus("error");
        }
    }, [draftId, getValues, media, router, selectedCategoryName]);

    const handleDiscard = useCallback(() => {
        reset({ ...initialFormValues, sku: generateSku() });
        setMedia([]);
        setMediaDirty(false);
        setShowUnsavedLeaveBanner(false);
        setSaveStatus("idle");
        toast.success("Unsaved changes discarded");
    }, [reset]);

    // ⌘+S shortcut
    useEffect(() => {
        const handler = (e: KeyboardEvent) => {
            if ((e.metaKey || e.ctrlKey) && e.key === "s") {
                e.preventDefault();
                handleSave();
            }
        };
        window.addEventListener("keydown", handler);
        return () => window.removeEventListener("keydown", handler);
    }, [handleSave]);

    return (
        <FormProvider {...methods}>
            <div className="space-y-6">
                {showUnsavedLeaveBanner && hasUnsavedChanges && (
                    <div className="sticky top-0 z-40 rounded-xl border border-amber-400/40 bg-amber-50/95 p-3 shadow-sm backdrop-blur dark:border-amber-500/30 dark:bg-amber-950/40">
                        <div className="flex animate-pulse items-center justify-between gap-3">
                            <p className="text-sm font-medium text-amber-900 dark:text-amber-200">
                                You have unsaved changes
                            </p>
                            <div className="flex items-center gap-2">
                                <Button size="sm" variant="outline" onClick={handleDiscard}>
                                    Discard
                                </Button>
                                <Button size="sm" onClick={() => { void handleSave(); }} disabled={saveStatus === "saving"}>
                                    Save
                                </Button>
                            </div>
                        </div>
                    </div>
                )}

                {/* Top bar */}
                <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                        <Button variant="ghost" size="icon" asChild>
                            <Link href="/products">
                                <ArrowLeft className="size-4" />
                            </Link>
                        </Button>
                        <div>
                            <h1 className="text-xl font-semibold">
                                {formVals.title || "New product"}
                            </h1>
                            <div className="flex items-center gap-2 text-xs text-[var(--muted-foreground)]">
                                <span>{formVals.status === "draft" ? "Draft · not published" : formVals.status}</span>
                                {saveStatus === "saving" && (
                                    <span className="flex items-center gap-1 text-blue-500">
                                        <Loader2 className="size-3 animate-spin" /> Saving...
                                    </span>
                                )}
                                {saveStatus === "saved" && (
                                    <span className="flex items-center gap-1 text-green-500">
                                        <CheckCircle2 className="size-3" /> Saved
                                    </span>
                                )}
                                {saveStatus === "error" && (
                                    <span className="text-red-500">Error saving</span>
                                )}
                            </div>
                        </div>
                    </div>
                    <div className="flex items-center gap-2">
                        <Button
                            variant="outline"
                            onClick={() => {
                                setValue("status", "draft", { shouldDirty: true });
                                void handleSave("draft");
                            }}
                            disabled={saveStatus === "saving"}
                        >
                            Save draft
                        </Button>
                        <Button
                            onClick={() => {
                                setValue("status", "active", { shouldDirty: true });
                                void handleSave("active");
                            }}
                            disabled={saveStatus === "saving"}
                        >
                            <Save className="size-4" />
                            Publish
                        </Button>
                    </div>
                </div>

                {/* Two-column layout */}
                <div className="grid gap-6 xl:grid-cols-[1fr_320px]">
                    {/* ── LEFT COLUMN ── */}
                    <div className="space-y-5">
                        {/* Core product information */}
                        <Card>
                            <CardHeader>
                                <CardTitle>Product information</CardTitle>
                            </CardHeader>
                            <CardContent className="space-y-5">
                                <div className="space-y-1.5">
                                    <Label>Title</Label>
                                    <Input
                                        placeholder="e.g. Cotton T-Shirt"
                                        {...register("title")}
                                    />
                                </div>
                                <SlugInput
                                    title={formVals.title || ""}
                                    value={formVals.slug || ""}
                                    onChange={(v) => setValue("slug", v, { shouldDirty: true })}
                                />
                                <div className="space-y-1.5">
                                    <Label>Description</Label>
                                    <RichTextEditor
                                        value={formVals.description || ""}
                                        onChange={(v) => setValue("description", v, { shouldDirty: true })}
                                    />
                                </div>
                                <div className="space-y-2">
                                    <Label>Media</Label>
                                    <div className="flex items-start gap-2">
                                        <div className="flex-1">
                                            <SortableMediaGrid
                                                items={media}
                                                onChange={(items) => {
                                                    setMedia(items);
                                                    setMediaDirty(true);
                                                }}
                                            />
                                        </div>
                                        <Button
                                            variant="outline"
                                            type="button"
                                            size="icon"
                                            onClick={() => setMediaOpen(true)}
                                            title="Add more images"
                                        >
                                            <ImagePlus className="size-4" />
                                        </Button>
                                    </div>
                                </div>
                                <div className="space-y-1.5">
                                    <Label>Category</Label>
                                    <CategorySelector
                                        value={formVals.categoryId}
                                        onChange={(id, category) => {
                                            setValue("categoryId", id, { shouldDirty: true });
                                            setSelectedCategoryName(category?.title || "");
                                        }}
                                    />
                                </div>
                                <div className="space-y-2 rounded-xl border border-[var(--border)] p-3">
                                    <div className="flex items-center justify-between">
                                        <Label>Track Inventory</Label>
                                        <button
                                            type="button"
                                            onClick={() => setValue("trackInventory", !formVals.trackInventory, { shouldDirty: true })}
                                            className={`relative inline-flex h-6 w-11 items-center rounded-full transition ${formVals.trackInventory ? "bg-emerald-600" : "bg-slate-300"}`}
                                        >
                                            <span className={`inline-block h-4 w-4 transform rounded-full bg-white transition ${formVals.trackInventory ? "translate-x-6" : "translate-x-1"}`} />
                                        </button>
                                    </div>
                                    {formVals.trackInventory && (
                                        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
                                            <div className="space-y-1.5">
                                                <Label>Quantity</Label>
                                                <Input type="number" min="0" {...register("quantity")} />
                                            </div>
                                            <div className="space-y-1.5">
                                                <Label>Warehouse</Label>
                                                <select
                                                    value={formVals.inventoryWarehouseId || ""}
                                                    onChange={(e) => setValue("inventoryWarehouseId", e.target.value || null, { shouldDirty: true })}
                                                    className="h-10 w-full rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3 text-sm"
                                                >
                                                    {warehouses.map((w) => (
                                                        <option key={w.id} value={w.id}>{w.name}</option>
                                                    ))}
                                                </select>
                                            </div>
                                        </div>
                                    )}
                                </div>
                            </CardContent>
                        </Card>

                        {/* Pricing */}
                        <Card>
                            <CardHeader>
                                <CardTitle>Pricing</CardTitle>
                            </CardHeader>
                            <CardContent className="space-y-4">
                                <div className="grid grid-cols-2 gap-3">
                                    <div className="space-y-1.5">
                                        <Label>Regular price (AED)</Label>
                                        <Input
                                            type="number"
                                            min="0"
                                            step="0.01"
                                            placeholder="0.00"
                                            {...register("price")}
                                        />
                                    </div>
                                    <div className="space-y-1.5">
                                        <Label>Sale price (AED)</Label>
                                        <Input
                                            type="number"
                                            min="0"
                                            step="0.01"
                                            placeholder="0.00"
                                            {...register("salePrice")}
                                        />
                                    </div>
                                </div>
                                <div className="space-y-1.5">
                                    <Label>Cost per item (AED)</Label>
                                    <Input
                                        type="number"
                                        min="0"
                                        step="0.01"
                                        placeholder="0.00"
                                        {...register("cost")}
                                    />
                                    <p className="text-xs text-[var(--muted-foreground)]">
                                        Used for FIFO COGS calculation — not shown to customers
                                    </p>
                                </div>
                                <label className="flex cursor-pointer items-center gap-2 text-sm">
                                    <input
                                        type="checkbox"
                                        className="accent-[var(--primary)]"
                                        checked={formVals.chargeTax}
                                        onChange={(e) => setValue("chargeTax", e.target.checked, { shouldDirty: true })}
                                    />
                                    Charge tax (5% UAE VAT)
                                </label>
                            </CardContent>
                        </Card>

                        {/* Inventory */}
                        {(!formVals.variants || formVals.variants.length === 0) && (
                            <Card>
                                <CardHeader>
                                    <CardTitle>Inventory</CardTitle>
                                </CardHeader>
                                <CardContent className="space-y-4">
                                    <div className="grid grid-cols-2 gap-3">
                                        <div className="space-y-1.5">
                                            <Label>SKU</Label>
                                            <Input
                                                placeholder="JS-XXXXXXXX"
                                                {...register("sku")}
                                            />
                                        </div>
                                        <div className="space-y-1.5 flex flex-col justify-end">
                                            <Label className="mb-2">Barcode (<span className="text-[var(--muted-foreground)]">Scannable SKU</span>)</Label>
                                            <div className="h-10 flex items-center overflow-hidden">
                                                {formVals.sku ? (
                                                    <Barcode value={formVals.sku} displayValue={false} height={30} width={1.5} margin={0} background="transparent" />
                                                ) : (
                                                    <span className="text-xs text-[var(--muted-foreground)] text-center w-full">Enter SKU to generate barcode</span>
                                                )}
                                            </div>
                                        </div>
                                    </div>
                                    <div className="space-y-1.5">
                                        <Label>Quantity</Label>
                                        <Input
                                            type="number"
                                            min="0"
                                            {...register("quantity")}
                                        />
                                    </div>
                                </CardContent>
                            </Card>
                        )}

                        {/* Shipping */}
                        <Card>
                            <CardHeader>
                                <CardTitle>Shipping</CardTitle>
                            </CardHeader>
                            <CardContent>
                                <div className="flex items-center gap-2">
                                    <div className="flex-1 space-y-1.5">
                                        <Label>Weight</Label>
                                        <Input
                                            type="number"
                                            min="0"
                                            step="0.01"
                                            placeholder="0.00"
                                            {...register("weight")}
                                        />
                                    </div>
                                    <div className="space-y-1.5">
                                        <Label>Unit</Label>
                                        <div className="flex h-10 items-center rounded-xl border border-[var(--border)] bg-[var(--muted)] px-3 text-sm">
                                            {formVals.weightUnit || "kg"}
                                        </div>
                                    </div>
                                </div>
                            </CardContent>
                        </Card>

                        {/* Variant builder */}
                        <VariantBuilder
                            options={(formVals.options as ProductOption[]) || []}
                            onOptionsChange={(v) => setValue("options", v, { shouldDirty: true })}
                            variants={(formVals.variants as unknown as ProductVariantDraft[]) || []}
                            onVariantsChange={(v) => setValue("variants", v as unknown as Array<Record<string, unknown>>, { shouldDirty: true })}
                            nextSku={generateSku}
                            basePrice={formVals.price || ""}
                            baseSalePrice={formVals.salePrice || ""}
                            baseCost={formVals.cost || ""}
                        />

                        {/* SEO */}
                        <SeoSection
                            title={formVals.seoTitle || ""}
                            description={formVals.seoDescription || ""}
                            urlHandle={formVals.slug || ""}
                            onTitleChange={(v) => setValue("seoTitle", v, { shouldDirty: true })}
                            onDescriptionChange={(v) => setValue("seoDescription", v, { shouldDirty: true })}
                        />
                    </div>

                    {/* ── RIGHT COLUMN ── */}
                    <div className="space-y-4">
                        <ProductStatusPanel
                            status={formVals.status || "active"}
                            onStatusChange={(v) => setValue("status", v as ProductStatus, { shouldDirty: true })}
                            tags={formVals.tags || []}
                            onTagsChange={(v) => setValue("tags", v, { shouldDirty: true })}
                        />
                    </div>
                </div>

                {/* Media modal */}
                <MediaLibraryModal
                    open={mediaOpen}
                    onOpenChange={setMediaOpen}
                    value={media}
                    onChange={(items) => {
                        setMedia(items);
                        setMediaDirty(true);
                    }}
                />

                {leaveDialogOpen && (
                    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 p-4">
                        <Card className="w-full max-w-md">
                            <CardHeader>
                                <CardTitle>Leave with unsaved changes?</CardTitle>
                            </CardHeader>
                            <CardContent className="space-y-4">
                                <p className="text-sm text-[var(--muted-foreground)]">
                                    You have unsaved changes. Save before leaving, or discard changes and continue.
                                </p>
                                <div className="flex justify-end gap-2">
                                    <Button variant="outline" onClick={() => setLeaveDialogOpen(false)}>
                                        Stay
                                    </Button>
                                    <Button
                                        variant="outline"
                                        onClick={() => {
                                            setLeaveDialogOpen(false);
                                            setPendingHref(null);
                                            handleDiscard();
                                            if (pendingHref) router.push(pendingHref);
                                        }}
                                    >
                                        Discard & Leave
                                    </Button>
                                    <Button
                                        onClick={async () => {
                                            await handleSave();
                                            setLeaveDialogOpen(false);
                                            setShowUnsavedLeaveBanner(false);
                                            if (pendingHref) router.push(pendingHref);
                                            setPendingHref(null);
                                        }}
                                    >
                                        Save & Leave
                                    </Button>
                                </div>
                            </CardContent>
                        </Card>
                    </div>
                )}
            </div>
        </FormProvider>
    );
}
