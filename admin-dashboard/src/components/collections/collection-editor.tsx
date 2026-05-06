"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowLeft, ImageIcon, ImagePlus, Save, Search, Trash2, X } from "lucide-react";

import { Button, Badge, Card, CardContent, CardHeader, CardTitle, Input, Label } from "@/components/ui/primitives";
import { SlugInput } from "@/components/products/slug-input";
import { MediaLibraryModal } from "@/components/media/media-library-modal";
import { CollectionProductPickerModal } from "@/components/collections/collection-product-picker-modal";
import { api, publicUploadUrl } from "@/lib/api-client";
import type { MediaAsset, ProductCollection, ProductListItem } from "@/types/api";

type CollectionEditorProps = {
    collectionId?: string;
};

export function CollectionEditor({ collectionId }: CollectionEditorProps) {
    const router = useRouter();
    const queryClient = useQueryClient();

    const [title, setTitle] = useState("");
    const [slug, setSlug] = useState("");
    const [description, setDescription] = useState("");
    const [mediaOpen, setMediaOpen] = useState(false);
    const [media, setMedia] = useState<MediaAsset[]>([]);

    const [pickerOpen, setPickerOpen] = useState(false);
    const [selectedProducts, setSelectedProducts] = useState<ProductListItem[]>([]);

    const initialized = useRef(false);
    const lastCollectionId = useRef<string | undefined>(undefined);

    useEffect(() => {
        if (lastCollectionId.current !== collectionId) {
            lastCollectionId.current = collectionId;
            initialized.current = false;
        }
    }, [collectionId]);

    const { data: existing, isLoading } = useQuery({
        queryKey: ["collection", collectionId],
        queryFn: () => api.getCollection(collectionId!),
        enabled: Boolean(collectionId),
    });

    useEffect(() => {
        if (existing && collectionId && !initialized.current) {
            initialized.current = true;
            setTitle(existing.title ?? "");
            setSlug(existing.slug ?? "");
            setDescription(existing.description ?? "");
            if (existing.image_url) {
                setMedia([
                    {
                        id: "banner",
                        url: existing.image_url,
                        mime_type: "image/*",
                        sort_order: 0,
                    } as MediaAsset,
                ]);
            } else {
                setMedia([]);
            }
            const pids = existing.product_ids ?? [];
            if (pids.length > 0) {
                void Promise.allSettled(pids.map((pid) => api.getProduct(pid))).then((results) => {
                    const mapped: ProductListItem[] = results.flatMap((r) => {
                        if (r.status !== "fulfilled") return [];
                        const row = r.value.product as typeof r.value.product & { title?: string; slug?: string };
                        const idStr = String(row.id);
                        return [
                            {
                                id: idStr,
                                product_id: idStr,
                                name: row.name ?? row.title ?? "Product",
                                slug: row.slug ?? "",
                                sku: "",
                                price: "0",
                                stock: 0,
                                status: row.status ?? "active",
                            },
                        ];
                    });
                    setSelectedProducts(mapped);
                });
            } else {
                setSelectedProducts([]);
            }
        }
    }, [existing, collectionId]);

    const saveMutation = useMutation({
        mutationFn: async (payload: Partial<ProductCollection>) =>
            collectionId ? api.updateCollection(collectionId, payload) : api.createCollection(payload),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["collections"] });
            queryClient.invalidateQueries({ queryKey: ["products"] });
            if (collectionId) {
                queryClient.invalidateQueries({ queryKey: ["collection", collectionId] });
            }
            toast.success(collectionId ? "Collection saved" : "Collection created");
            router.push("/collections");
        },
        onError: (err) => toast.error(err instanceof Error ? err.message : "Failed to save"),
    });

    const deleteMutation = useMutation({
        mutationFn: () => api.deleteCollection(collectionId!),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["collections"] });
            toast.success("Collection deleted");
            router.push("/collections");
        },
        onError: (err) => toast.error(err instanceof Error ? err.message : "Failed to delete"),
    });

    const handleSave = useCallback(() => {
        if (!title.trim()) {
            toast.error("Name is required");
            return;
        }
        saveMutation.mutate({
            title: title.trim(),
            slug: slug.trim() || title.toLowerCase().replace(/\s+/g, "-"),
            description: description.trim() ? description.trim() : null,
            image_url: media.length > 0 ? media[0].url : null,
            product_ids: selectedProducts.map((p) => p.product_id),
        } as Partial<ProductCollection>);
    }, [title, slug, description, media, selectedProducts, saveMutation]);

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

    const handleRemoveProduct = (productId: string) => {
        setSelectedProducts(selectedProducts.filter((p) => p.product_id !== productId));
    };

    if (collectionId && isLoading) {
        return (
            <div className="space-y-4">
                {[...Array(4)].map((_, i) => (
                    <div key={i} className="h-16 animate-pulse rounded-xl bg-[var(--muted)]" />
                ))}
            </div>
        );
    }

    return (
        <div className="space-y-6">
            <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="flex items-center gap-3">
                    <Button variant="ghost" size="icon" asChild>
                        <Link href="/collections">
                            <ArrowLeft className="size-4" />
                        </Link>
                    </Button>
                    <div>
                        <h1 className="text-xl font-semibold">{title || "New collection"}</h1>
                        <p className="text-xs text-[var(--muted-foreground)]">Curated grouping — storefront uses /collection/… URLs</p>
                    </div>
                </div>
                <div className="flex gap-2">
                    {collectionId ? (
                        <Button
                            variant="outline"
                            onClick={() => {
                                if (confirm("Delete this collection? Products are not deleted.")) {
                                    deleteMutation.mutate();
                                }
                            }}
                            loading={deleteMutation.isPending}
                        >
                            <Trash2 className="size-4" /> Delete
                        </Button>
                    ) : null}
                    <Button loading={saveMutation.isPending} onClick={handleSave}>
                        <Save className="size-4" /> Save
                    </Button>
                </div>
            </div>

            <div className="grid gap-6 xl:grid-cols-[1fr_380px]">
                <div className="space-y-5">
                    <Card className="overflow-hidden">
                        <CardHeader className="border-b border-[var(--border)] pb-3">
                            <CardTitle className="text-base">Banner</CardTitle>
                            <p className="text-xs font-normal text-[var(--muted-foreground)]">Optional hero shown on collection landing pages</p>
                        </CardHeader>
                        <CardContent className="space-y-4 pt-5">
                            <div className="relative aspect-[21/9] w-full overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--muted)]/30">
                                {media[0] ? (
                                    <img src={publicUploadUrl(media[0].url)} alt="" className="size-full object-cover" />
                                ) : (
                                    <div className="flex size-full flex-col items-center justify-center gap-2 text-[var(--muted-foreground)]">
                                        <ImageIcon className="size-10 opacity-60" />
                                        <span className="text-xs">No banner selected</span>
                                    </div>
                                )}
                            </div>
                            <div className="flex flex-wrap gap-2">
                                <Button type="button" variant="outline" size="sm" onClick={() => setMediaOpen(true)}>
                                    <ImagePlus className="size-4" /> {media[0] ? "Change image" : "Choose image"}
                                </Button>
                                {media[0] ? (
                                    <Button type="button" variant="ghost" size="sm" onClick={() => setMedia([])}>
                                        Remove
                                    </Button>
                                ) : null}
                            </div>
                        </CardContent>
                    </Card>

                    <Card>
                        <CardHeader>
                            <CardTitle className="text-base">Basics</CardTitle>
                            <p className="text-xs font-normal text-[var(--muted-foreground)]">Name first — URL handle stays in sync unless you unlock it.</p>
                        </CardHeader>
                        <CardContent className="space-y-5">
                            <div className="space-y-1.5">
                                <Label>Name</Label>
                                <Input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="e.g. Summer essentials" />
                            </div>
                            <SlugInput
                                title={title}
                                value={slug}
                                onChange={setSlug}
                                permalinkPrefix="/collection/"
                                label="Storefront path"
                                helpAuto="Generated from name — unlock to customize the URL handle."
                                helpLocked="Custom URL — click lock to follow the name again."
                            />
                        </CardContent>
                    </Card>

                    <Card>
                        <CardHeader>
                            <CardTitle className="text-base">Description</CardTitle>
                            <p className="text-xs font-normal text-[var(--muted-foreground)]">Optional — shown below the banner on the storefront.</p>
                        </CardHeader>
                        <CardContent>
                            <textarea
                                rows={5}
                                className="min-h-[120px] w-full rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm outline-none ring-[var(--ring)]/40 focus-visible:ring-2"
                                value={description}
                                onChange={(e) => setDescription(e.target.value)}
                                placeholder="Short editorial blurb or promo copy…"
                            />
                        </CardContent>
                    </Card>
                    <MediaLibraryModal open={mediaOpen} onOpenChange={setMediaOpen} mode="single" value={media} onChange={setMedia} />
                </div>

                <div>
                    <Card>
                        <CardHeader>
                            <div className="flex items-center justify-between gap-2">
                                <CardTitle className="text-base">Products</CardTitle>
                                {selectedProducts.length > 0 ? (
                                    <Badge tone="success" className="tabular-nums">
                                        {selectedProducts.length} selected
                                    </Badge>
                                ) : null}
                            </div>
                            <p className="text-xs font-normal text-[var(--muted-foreground)]">
                                Browse the catalog in a larger window — products only (no variant rows).
                            </p>
                        </CardHeader>
                        <CardContent className="space-y-4">
                            <Button type="button" variant="outline" className="w-full gap-2" onClick={() => setPickerOpen(true)}>
                                <Search className="size-4" />
                                Browse and add products
                            </Button>
                            <CollectionProductPickerModal
                                open={pickerOpen}
                                onOpenChange={setPickerOpen}
                                initialSelection={selectedProducts}
                                onApply={setSelectedProducts}
                            />
                            <div className="space-y-2">
                                {selectedProducts.length === 0 ? (
                                    <p className="rounded-xl border border-dashed border-[var(--border)] bg-[var(--muted)]/20 px-3 py-6 text-center text-sm text-[var(--muted-foreground)]">
                                        No products yet. Tap <span className="font-medium text-[var(--foreground)]">Browse and add products</span> to
                                        search and multi-select.
                                    </p>
                                ) : (
                                    selectedProducts.map((p) => (
                                        <div
                                            key={p.product_id}
                                            className="flex items-center gap-3 rounded-xl border border-[var(--border)] px-3 py-2.5"
                                        >
                                            {p.thumbnail ? (
                                                <img
                                                    src={publicUploadUrl(p.thumbnail)}
                                                    alt=""
                                                    className="size-11 shrink-0 rounded-lg border border-[var(--border)] object-cover"
                                                />
                                            ) : (
                                                <div className="flex size-11 shrink-0 items-center justify-center rounded-lg border border-dashed border-[var(--border)] bg-[var(--muted)]/40">
                                                    <ImageIcon className="size-5 text-[var(--muted-foreground)]" />
                                                </div>
                                            )}
                                            <div className="min-w-0 flex-1">
                                                <p className="truncate text-sm font-medium">{p.name}</p>
                                                <p className="truncate text-xs text-[var(--muted-foreground)]">
                                                    AED {Number.parseFloat(String(p.price || "0")).toFixed(2)} · Qty {p.stock ?? 0}
                                                </p>
                                            </div>
                                            <Button
                                                type="button"
                                                variant="ghost"
                                                size="icon"
                                                aria-label={`Remove ${p.name}`}
                                                onClick={() => handleRemoveProduct(p.product_id)}
                                            >
                                                <X className="size-4" />
                                            </Button>
                                        </div>
                                    ))
                                )}
                            </div>
                        </CardContent>
                    </Card>
                </div>
            </div>
        </div>
    );
}
