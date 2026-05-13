"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import * as Dialog from "@radix-ui/react-dialog";
import { useInfiniteQuery } from "@tanstack/react-query";
import { Check, ImageIcon, Loader2, Search, X } from "lucide-react";

import { Button, Input, Label } from "@/components/ui/primitives";
import { api, listThumbUrl } from "@/lib/api-client";
import { cn } from "@/lib/utils";
import type { ProductListItem } from "@/types/api";

function useDebounce<T>(value: T, delay: number): T {
    const [debouncedValue, setDebouncedValue] = useState<T>(value);
    useEffect(() => {
        const t = setTimeout(() => setDebouncedValue(value), delay);
        return () => clearTimeout(t);
    }, [value, delay]);
    return debouncedValue;
}

function formatMoney(amount: string | number): string {
    const n = typeof amount === "string" ? parseFloat(amount) : amount;
    if (!Number.isFinite(n)) return "—";
    return n.toFixed(2);
}

type CollectionProductPickerModalProps = {
    open: boolean;
    onOpenChange: (open: boolean) => void;
    /** Current selection (product-level rows). */
    initialSelection: ProductListItem[];
    onApply: (next: ProductListItem[]) => void;
};

export function CollectionProductPickerModal({
    open,
    onOpenChange,
    initialSelection,
    onApply,
}: CollectionProductPickerModalProps) {
    const [search, setSearch] = useState("");
    const debouncedSearch = useDebounce(search, 280);
    const [picked, setPicked] = useState<Map<string, ProductListItem>>(new Map());
    const sentinelRef = useRef<HTMLDivElement | null>(null);
    const latestInitialRef = useRef(initialSelection);
    latestInitialRef.current = initialSelection;

    useEffect(() => {
        if (!open) return;
        const m = new Map<string, ProductListItem>();
        latestInitialRef.current.forEach((p) => {
            const id = String(p.product_id);
            m.set(id, { ...p, product_id: id, id: p.id || id });
        });
        setPicked(m);
        setSearch("");
    }, [open]);

    const query = useInfiniteQuery({
        queryKey: ["collection-product-picker", debouncedSearch.trim()],
        queryFn: async ({ pageParam }) => {
            const page = typeof pageParam === "number" ? pageParam : 1;
            return api.listProducts({
                page,
                page_size: 40,
                search: debouncedSearch.trim() || undefined,
                inventory: "product",
            });
        },
        initialPageParam: 1,
        getNextPageParam: (lastPage, allPages) => {
            const loaded = allPages.reduce((acc, p) => acc + p.items.length, 0);
            if (loaded >= lastPage.total) return undefined;
            return allPages.length + 1;
        },
        enabled: open,
        staleTime: 30_000,
    });

    const rows = useMemo(() => {
        const pages = query.data?.pages ?? [];
        const byProduct = new Map<string, ProductListItem>();
        for (const page of pages) {
            for (const row of page.items) {
                const pid = String(row.product_id);
                if (!byProduct.has(pid)) {
                    byProduct.set(pid, {
                        ...row,
                        product_id: pid,
                        id: String(row.id),
                    });
                }
            }
        }
        return Array.from(byProduct.values());
    }, [query.data?.pages]);

    const { fetchNextPage, hasNextPage, isFetchingNextPage } = query;

    useEffect(() => {
        if (!open || !sentinelRef.current) return;
        const el = sentinelRef.current;
        const obs = new IntersectionObserver(
            (entries) => {
                const hit = entries.some((e) => e.isIntersecting);
                if (hit && hasNextPage && !isFetchingNextPage) void fetchNextPage();
            },
            { root: null, rootMargin: "120px", threshold: 0 },
        );
        obs.observe(el);
        return () => obs.disconnect();
    }, [open, hasNextPage, isFetchingNextPage, fetchNextPage, rows.length]);

    const toggle = useCallback((row: ProductListItem) => {
        const pid = String(row.product_id);
        setPicked((prev) => {
            const next = new Map(prev);
            if (next.has(pid)) next.delete(pid);
            else
                next.set(pid, {
                    ...row,
                    product_id: pid,
                    id: String(row.id),
                });
            return next;
        });
    }, []);

    const handleApply = () => {
        onApply(Array.from(picked.values()));
        onOpenChange(false);
    };

    const loading = query.isPending || query.isFetching;

    return (
        <Dialog.Root open={open} onOpenChange={onOpenChange}>
            <Dialog.Portal>
                <Dialog.Overlay className="fixed inset-0 z-[100] bg-black/50 backdrop-blur-[2px] data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0" />
                <Dialog.Content
                    className={cn(
                        "fixed left-1/2 top-1/2 z-[101] flex max-h-[min(88vh,820px)] w-[min(96vw,720px)] -translate-x-1/2 -translate-y-1/2 flex-col overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--panel)] shadow-2xl",
                        "data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0 data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95",
                    )}
                    onOpenAutoFocus={(e) => e.preventDefault()}
                >
                    <Dialog.Title className="sr-only">Add products to collection</Dialog.Title>
                    <Dialog.Description className="sr-only">
                        Search the catalog and select one or more products. Stock is total across variants.
                    </Dialog.Description>

                    <div className="flex items-start justify-between gap-3 border-b border-[var(--border)] px-5 py-4">
                        <div>
                            <h2 className="text-lg font-semibold">Add products</h2>
                            <p className="mt-0.5 text-xs text-[var(--muted-foreground)]">
                                One row per product · thumbnail & price from primary variant · stock = sum across variants
                            </p>
                        </div>
                        <Dialog.Close asChild>
                            <Button variant="ghost" size="icon" aria-label="Close">
                                <X className="size-4" />
                            </Button>
                        </Dialog.Close>
                    </div>

                    <div className="border-b border-[var(--border)] px-5 py-3">
                        <Label className="sr-only" htmlFor="collection-picker-search">
                            Search products
                        </Label>
                        <div className="relative">
                            <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-[var(--muted-foreground)]" />
                            <Input
                                id="collection-picker-search"
                                className="pl-10 pr-10"
                                placeholder="Search by product name or SKU…"
                                value={search}
                                onChange={(e) => setSearch(e.target.value)}
                                autoFocus
                            />
                            {loading ? (
                                <Loader2 className="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 animate-spin text-[var(--muted-foreground)]" />
                            ) : null}
                        </div>
                    </div>

                    <div className="min-h-0 flex-1 overflow-y-auto px-2 py-2">
                        {query.isError ? (
                            <p className="px-3 py-8 text-center text-sm text-rose-600">Could not load products.</p>
                        ) : rows.length === 0 && !loading ? (
                            <p className="px-3 py-10 text-center text-sm text-[var(--muted-foreground)]">
                                {debouncedSearch.trim() ? "No products match your search." : "No products in catalog."}
                            </p>
                        ) : (
                            <ul className="space-y-1">
                                {rows.map((row) => {
                                    const pid = String(row.product_id);
                                    const checked = picked.has(pid);
                                    return (
                                        <li key={pid}>
                                            <button
                                                type="button"
                                                onClick={() => toggle(row)}
                                                className={cn(
                                                    "flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left transition",
                                                    checked
                                                        ? "bg-[var(--muted)]/80 ring-1 ring-[var(--ring)]/40"
                                                        : "hover:bg-[var(--muted)]/50",
                                                )}
                                            >
                                                <span
                                                    className={cn(
                                                        "flex size-5 shrink-0 items-center justify-center rounded border border-[var(--border)]",
                                                        checked && "border-[var(--primary)] bg-[var(--primary)] text-white",
                                                    )}
                                                >
                                                    {checked ? <Check className="size-3" /> : null}
                                                </span>
                                                {row.thumbnail ? (
                                                    <img
                                                        src={listThumbUrl(row.thumbnail, 160)}
                                                        alt=""
                                                        className="size-12 shrink-0 rounded-lg border border-[var(--border)] object-cover"
                                                    />
                                                ) : (
                                                    <div className="flex size-12 shrink-0 items-center justify-center rounded-lg border border-dashed border-[var(--border)] bg-[var(--muted)]/40">
                                                        <ImageIcon className="size-5 text-[var(--muted-foreground)]" />
                                                    </div>
                                                )}
                                                <div className="min-w-0 flex-1">
                                                    <p className="truncate font-medium">{row.name}</p>
                                                    <p className="truncate text-xs text-[var(--muted-foreground)]">
                                                        AED {formatMoney(row.price)} · Qty {row.stock}
                                                    </p>
                                                </div>
                                            </button>
                                        </li>
                                    );
                                })}
                            </ul>
                        )}
                        <div ref={sentinelRef} className="h-4" aria-hidden />
                        {query.isFetchingNextPage ? (
                            <p className="py-2 text-center text-xs text-[var(--muted-foreground)]">Loading more…</p>
                        ) : null}
                    </div>

                    <div className="flex items-center justify-between gap-3 border-t border-[var(--border)] px-5 py-3">
                        <p className="text-xs text-[var(--muted-foreground)]">
                            <span className="font-medium text-[var(--foreground)]">{picked.size}</span> selected
                        </p>
                        <div className="flex gap-2">
                            <Dialog.Close asChild>
                                <Button type="button" variant="outline">
                                    Cancel
                                </Button>
                            </Dialog.Close>
                            <Button type="button" onClick={handleApply}>
                                Apply selection
                            </Button>
                        </div>
                    </div>
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog.Root>
    );
}
