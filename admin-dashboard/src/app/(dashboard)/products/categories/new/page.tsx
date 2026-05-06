"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { ArrowLeft, ImagePlus, Save, Plus, X, Search } from "lucide-react";
import Link from "next/link";

import { Button, Card, CardContent, CardHeader, CardTitle, Input, Label, Badge } from "@/components/ui/primitives";
import { RichTextEditor } from "@/components/products/rich-text-editor";
import { SlugInput } from "@/components/products/slug-input";
import { MediaLibraryModal } from "@/components/media/media-library-modal";
import { api, publicUploadUrl } from "@/lib/api-client";
import type { MediaAsset, ProductCategory, SmartCollectionCondition, ProductListItem } from "@/types/api";

function useDebounce<T>(value: T, delay: number): T {
    const [debouncedValue, setDebouncedValue] = useState<T>(value);
    useEffect(() => {
        const handler = setTimeout(() => setDebouncedValue(value), delay);
        return () => clearTimeout(handler);
    }, [value, delay]);
    return debouncedValue;
}

export default function NewCategoryPage() {
    const router = useRouter();
    const queryClient = useQueryClient();

    const [title, setTitle] = useState("");
    const [slug, setSlug] = useState("");
    const [description, setDescription] = useState("");
    const [type, setType] = useState<"manual" | "smart">("manual");
    const [conditions, setConditions] = useState<SmartCollectionCondition[]>([
        { field: "title", operator: "contains", value: "" },
    ]);
    const [mediaOpen, setMediaOpen] = useState(false);
    const [media, setMedia] = useState<MediaAsset[]>([]);

    // Manual mode specific: product assignment
    const [searchQuery, setSearchQuery] = useState("");
    const debouncedSearch = useDebounce(searchQuery, 300);
    const [selectedProducts, setSelectedProducts] = useState<ProductListItem[]>([]);

    // Unsaved changes guard
    const isDirty = useRef(false);
    useEffect(() => {
        if (title || description) isDirty.current = true;
    }, [title, description]);
    useEffect(() => {
        const onBeforeUnload = (e: BeforeUnloadEvent) => {
            if (isDirty.current) e.preventDefault();
        };
        window.addEventListener("beforeunload", onBeforeUnload);
        return () => window.removeEventListener("beforeunload", onBeforeUnload);
    }, []);

    const { data: searchResults, isLoading: searching } = useQuery({
        queryKey: ["products-search", debouncedSearch],
        queryFn: () => api.listProducts({ page: 1, page_size: 10, search: debouncedSearch }),
        enabled: type === "manual" && debouncedSearch.length > 0,
    });

    const productsList = searchResults?.items ?? [];

    const mutation = useMutation({
        mutationFn: (payload: Partial<ProductCategory>) => api.createCategory(payload),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["categories"] });
            isDirty.current = false;
            toast.success("Category created");
            router.push("/products/categories");
        },
        onError: (err) =>
            toast.error(err instanceof Error ? err.message : "Failed to create category"),
    });

    const handleSave = useCallback(() => {
        if (!title.trim()) {
            toast.error("Title is required");
            return;
        }

        const payload: Partial<ProductCategory> = {
            title: title.trim(),
            slug: slug || title.toLowerCase().replace(/\s+/g, "-"),
            description,
            type,
            image_url: media.length > 0 ? media[0].url : null,
        };

        if (type === "smart") {
            // Filter out empty conditions securely
            payload.conditions = conditions.filter(c => c.value.trim().length > 0);
            if (payload.conditions.length === 0) {
                toast.error("Smart categories must have at least one valid condition.");
                return;
            }
        } else {
            payload.product_ids = selectedProducts.map((p) => p.product_id);
        }

        mutation.mutate(payload);
    }, [title, slug, description, type, conditions, selectedProducts, media, mutation]);

    // ⌘+S / Ctrl+S shortcut
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

    const handleAddCondition = () => {
        setConditions([...conditions, { field: "title", operator: "contains", value: "" }]);
    };

    const handleUpdateCondition = (index: number, changes: Partial<SmartCollectionCondition>) => {
        const next = [...conditions];
        next[index] = { ...next[index], ...changes };
        setConditions(next);
    };

    const handleRemoveCondition = (index: number) => {
        setConditions(conditions.filter((_, i) => i !== index));
    };

    const handleAddProduct = (product: ProductListItem) => {
        if (!selectedProducts.find((p) => p.product_id === product.product_id)) {
            setSelectedProducts([...selectedProducts, product]);
        }
        setSearchQuery("");
    };

    const handleRemoveProduct = (productId: string) => {
        setSelectedProducts(selectedProducts.filter((p) => p.product_id !== productId));
    };

    return (
        <div className="space-y-6">
            {/* Top bar */}
            <div className="flex items-center justify-between">
                <div className="flex items-center gap-3">
                    <Button variant="ghost" size="icon" asChild>
                        <Link href="/products/categories">
                            <ArrowLeft className="size-4" />
                        </Link>
                    </Button>
                    <div>
                        <h1 className="text-xl font-semibold">
                            {title || "New category"}
                        </h1>
                    </div>
                </div>
                <div className="flex items-center gap-2">
                    <Button
                        loading={mutation.isPending}
                        onClick={handleSave}
                    >
                        <Save className="size-4" />
                        Save
                    </Button>
                </div>
            </div>

            {/* Two-column layout matching products */}
            <div className="grid gap-6 xl:grid-cols-[1fr_320px]">
                {/* ── LEFT COLUMN ── */}
                <div className="space-y-5">
                    <Card>
                        <CardContent className="space-y-4 pt-5">
                            <div className="space-y-1.5">
                                <Label>Title</Label>
                                <Input
                                    placeholder="e.g. Summer Collection"
                                    value={title}
                                    onChange={(e) => setTitle(e.target.value)}
                                />
                            </div>
                            <SlugInput title={title} value={slug} onChange={setSlug} />
                        </CardContent>
                    </Card>

                    <Card>
                        <CardHeader>
                            <CardTitle>Description</CardTitle>
                        </CardHeader>
                        <CardContent>
                            <RichTextEditor value={description} onChange={setDescription} />
                        </CardContent>
                    </Card>

                    <Card>
                        <CardHeader>
                            <CardTitle>Category type</CardTitle>
                        </CardHeader>
                        <CardContent className="space-y-6">
                            <div className="space-y-3">
                                <label className="flex items-start gap-3 rounded-lg border border-[var(--border)] p-4 cursor-pointer hover:bg-[var(--muted)]/50 transition-colors">
                                    <input
                                        type="radio"
                                        name="collection_type"
                                        className="mt-1 accent-[var(--primary)]"
                                        checked={type === "manual"}
                                        onChange={() => setType("manual")}
                                    />
                                    <div>
                                        <div className="font-medium">Manual</div>
                                        <div className="text-sm text-[var(--muted-foreground)]">
                                            Add products to this category one by one.
                                        </div>
                                    </div>
                                </label>
                                <label className="flex items-start gap-3 rounded-lg border border-[var(--border)] p-4 cursor-pointer hover:bg-[var(--muted)]/50 transition-colors">
                                    <input
                                        type="radio"
                                        name="collection_type"
                                        className="mt-1 accent-[var(--primary)]"
                                        checked={type === "smart"}
                                        onChange={() => setType("smart")}
                                    />
                                    <div>
                                        <div className="font-medium">Smart</div>
                                        <div className="text-sm text-[var(--muted-foreground)]">
                                            Products that match the conditions you set will automatically be added.
                                        </div>
                                    </div>
                                </label>
                            </div>

                            {type === "smart" && (
                                <div className="space-y-4 pt-2 border-t border-[var(--border)]">
                                    <Label>Conditions</Label>
                                    <div className="space-y-3">
                                        {conditions.map((condition, index) => (
                                            <div key={index} className="flex items-center gap-2">
                                                <select
                                                    value={condition.field}
                                                    onChange={(e) => handleUpdateCondition(index, { field: e.target.value as any })}
                                                    className="rounded-lg border border-[var(--border)] bg-[var(--background)] px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ring)]/30 w-1/3"
                                                >
                                                    <option value="title">Product title</option>
                                                    <option value="price">Price</option>
                                                    <option value="tag">Product tag</option>
                                                    <option value="inventory">Inventory stock</option>
                                                </select>
                                                <select
                                                    value={condition.operator}
                                                    onChange={(e) => handleUpdateCondition(index, { operator: e.target.value as any })}
                                                    className="rounded-lg border border-[var(--border)] bg-[var(--background)] px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-[var(--ring)]/30 w-1/3"
                                                >
                                                    <option value="equals">is equal to</option>
                                                    <option value="contains">contains</option>
                                                    <option value="greater_than">is greater than</option>
                                                    <option value="less_than">is less than</option>
                                                </select>
                                                <Input
                                                    className="flex-1"
                                                    placeholder="Value"
                                                    value={condition.value}
                                                    onChange={(e) => handleUpdateCondition(index, { value: e.target.value })}
                                                />
                                                {conditions.length > 1 && (
                                                    <Button variant="ghost" size="icon" onClick={() => handleRemoveCondition(index)}>
                                                        <X className="size-4 text-[var(--muted-foreground)]" />
                                                    </Button>
                                                )}
                                            </div>
                                        ))}
                                    </div>
                                    <Button type="button" variant="outline" size="sm" onClick={handleAddCondition}>
                                        <Plus className="size-4" /> Add another condition
                                    </Button>
                                </div>
                            )}
                        </CardContent>
                    </Card>

                    {/* Conditional Products assignment block explicitly required by user */}
                    {type === "manual" && (
                        <Card>
                            <CardHeader>
                                <CardTitle>Products</CardTitle>
                            </CardHeader>
                            <CardContent className="space-y-4">
                                <div className="relative">
                                    <Search className="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-[var(--muted-foreground)]" />
                                    <Input
                                        className="pl-9"
                                        placeholder="Search products to add..."
                                        value={searchQuery}
                                        onChange={(e) => setSearchQuery(e.target.value)}
                                    />
                                    {searchQuery.length > 0 && productsList.length > 0 && (
                                        <div className="absolute z-10 mt-1 w-full rounded-lg border border-[var(--border)] bg-[var(--panel)] shadow-lg overflow-hidden max-h-[300px] overflow-y-auto">
                                            {productsList.map((product: ProductListItem) => (
                                                <button
                                                    key={product.id}
                                                    className="w-full flex items-center gap-3 px-4 py-3 hover:bg-[var(--muted)] text-left"
                                                    onClick={() => handleAddProduct(product)}
                                                >
                                                    <div className="size-8 rounded bg-[var(--border)] overflow-hidden shrink-0">
                                                        <div className="w-full h-full bg-[var(--muted-foreground)]/10" />
                                                    </div>
                                                    <div className="flex-1 overflow-hidden">
                                                        <p className="text-sm font-medium truncate">{product.name}</p>
                                                    </div>
                                                    <Badge>Add</Badge>
                                                </button>
                                            ))}
                                        </div>
                                    )}
                                </div>

                                {selectedProducts.length === 0 ? (
                                    <div className="text-center py-8 text-sm text-[var(--muted-foreground)] border-t border-[var(--border)]">
                                        There are no products in this collection.
                                    </div>
                                ) : (
                                    <div className="space-y-1 border-t border-[var(--border)] pt-4">
                                        {selectedProducts.map((product: ProductListItem) => (
                                            <div key={product.id} className="flex items-center justify-between p-2 hover:bg-[var(--muted)] rounded-lg group">
                                                <span className="text-sm font-medium">{product.name}</span>
                                                <button
                                                    onClick={() => handleRemoveProduct(product.product_id)}
                                                    className="text-[var(--muted-foreground)] hover:text-red-500 opacity-0 group-hover:opacity-100 transition-opacity"
                                                >
                                                    <X className="size-4" />
                                                </button>
                                            </div>
                                        ))}
                                    </div>
                                )}
                            </CardContent>
                        </Card>
                    )}
                </div>

                {/* ── RIGHT COLUMN ── */}
                <div className="space-y-4">
                    <Card>
                        <CardHeader>
                            <CardTitle>Category Media</CardTitle>
                        </CardHeader>
                        <CardContent>
                            {media.length > 0 ? (
                                <div className="space-y-4">
                                    <div className="aspect-[4/3] w-full overflow-hidden rounded-lg border border-[var(--border)]">
                                        <img
                                            src={publicUploadUrl(media[0].url)}
                                            alt={media[0].alt || "Category Media"}
                                            className="h-full w-full object-cover"
                                        />
                                    </div>
                                    <div className="flex gap-2">
                                        <Button variant="outline" className="flex-1" onClick={() => setMediaOpen(true)}>
                                            Change
                                        </Button>
                                    </div>
                                </div>
                            ) : (
                                <div
                                    className="flex aspect-[4/3] w-full flex-col items-center justify-center gap-2 rounded-lg border border-dashed border-[var(--border)] bg-[var(--muted)]/50 cursor-pointer hover:bg-[var(--muted)] transition-colors text-sm text-[var(--muted-foreground)]"
                                    onClick={() => setMediaOpen(true)}
                                >
                                    <ImagePlus className="size-6 opacity-80" />
                                    Add image
                                </div>
                            )}
                        </CardContent>
                    </Card>
                </div>
            </div>

            {/* Media picker explicitly restricting out multiple files internally handled */}
            <MediaLibraryModal
                open={mediaOpen}
                onOpenChange={setMediaOpen}
                value={media}
                onChange={(newMedia) => setMedia(newMedia.slice(0, 1))}
            />
        </div>
    );
}
