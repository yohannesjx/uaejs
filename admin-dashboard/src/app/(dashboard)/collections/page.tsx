"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { ImageIcon, Layers, Pencil, Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Badge, Button } from "@/components/ui/primitives";
import { api, publicUploadUrl } from "@/lib/api-client";

export default function CollectionsPage() {
    const queryClient = useQueryClient();
    const { data: collections = [], isLoading } = useQuery({
        queryKey: ["collections"],
        queryFn: () => api.listCollections(),
        staleTime: 60_000,
        refetchOnWindowFocus: false,
        refetchOnReconnect: false,
    });

    const deleteMutation = useMutation({
        mutationFn: (id: string) => api.deleteCollection(id),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ["collections"] });
            toast.success("Collection deleted");
        },
        onError: (err) => toast.error(err instanceof Error ? err.message : "Delete failed"),
    });

    return (
        <div className="space-y-6">
            <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div>
                    <h1 className="text-xl font-semibold">Collections</h1>
                    <p className="mt-1 text-sm text-[var(--muted-foreground)]">
                        Flexible merchandising groups (trending, seasonal, landing pages). Independent from product categories.
                    </p>
                </div>
                <Button asChild className="shrink-0">
                    <Link href="/collections/new">
                        <Plus className="size-4" /> Create collection
                    </Link>
                </Button>
            </div>

            {isLoading ? (
                <div className="space-y-2">
                    {[...Array(4)].map((_, i) => (
                        <div key={i} className="h-[4.75rem] animate-pulse rounded-xl bg-[var(--muted)]" />
                    ))}
                </div>
            ) : collections.length === 0 ? (
                <div className="flex min-h-[360px] flex-col items-center justify-center rounded-2xl border border-dashed border-[var(--border)] bg-[var(--panel)] px-6 py-14 text-center">
                    <div className="mb-5 flex size-14 items-center justify-center rounded-full border border-[var(--border)] bg-[var(--muted)]/40">
                        <Layers className="size-7 text-[var(--muted-foreground)]" />
                    </div>
                    <p className="text-lg font-medium">No collections yet</p>
                    <p className="mx-auto mt-2 max-w-md text-sm text-[var(--muted-foreground)]">
                        Bundle products for homepage sections, drops, or promos — one product can belong to many collections without changing categories.
                    </p>
                    <Button asChild className="mt-8">
                        <Link href="/collections/new">
                            <Plus className="size-4" /> Create your first collection
                        </Link>
                    </Button>
                </div>
            ) : (
                <ul className="space-y-2" aria-label="Collections">
                    {collections.map((c) => {
                        const count = c.product_count ?? 0;
                        const deleting = deleteMutation.isPending && deleteMutation.variables === c.id;
                        return (
                            <li
                                key={c.id}
                                className="flex flex-col overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--panel)] sm:flex-row sm:items-stretch"
                            >
                                <Link
                                    href={`/collections/${c.id}/edit`}
                                    className="flex min-w-0 flex-1 items-center gap-3 p-3 transition hover:bg-[var(--muted)]/35 sm:p-4"
                                >
                                    {c.image_url ? (
                                        <img
                                            src={publicUploadUrl(c.image_url)}
                                            alt=""
                                            className="size-14 shrink-0 rounded-lg border border-[var(--border)] object-cover sm:size-16"
                                        />
                                    ) : (
                                        <div className="flex size-14 shrink-0 items-center justify-center rounded-lg border border-dashed border-[var(--border)] bg-[var(--muted)]/40 sm:size-16">
                                            <ImageIcon className="size-6 text-[var(--muted-foreground)]" />
                                        </div>
                                    )}
                                    <div className="min-w-0 flex-1 text-left">
                                        <p className="truncate font-medium">{c.title}</p>
                                        <p className="mt-0.5 text-xs text-[var(--muted-foreground)]">/{c.slug}</p>
                                        <div className="mt-2 flex flex-wrap items-center gap-2">
                                            <Badge tone={count === 0 ? "warning" : "success"} className="tabular-nums">
                                                {count} {count === 1 ? "product" : "products"}
                                            </Badge>
                                            {count === 0 ? (
                                                <span className="text-[11px] text-[var(--muted-foreground)]">No products assigned</span>
                                            ) : null}
                                        </div>
                                    </div>
                                </Link>
                                <div className="flex shrink-0 items-center justify-end gap-1 border-t border-[var(--border)] px-2 py-2 sm:border-l sm:border-t-0 sm:px-2">
                                    <Button variant="outline" size="icon" title="Edit" asChild className="shrink-0">
                                        <Link href={`/collections/${c.id}/edit`} aria-label={`Edit ${c.title}`}>
                                            <Pencil className="size-4" />
                                        </Link>
                                    </Button>
                                    <Button
                                        variant="outline"
                                        size="icon"
                                        title="Delete"
                                        loading={deleting}
                                        disabled={deleteMutation.isPending}
                                        className="shrink-0 text-[var(--foreground)] hover:text-rose-600"
                                        aria-label={`Delete ${c.title}`}
                                        onClick={() => {
                                            if (
                                                !confirm(
                                                    `Delete "${c.title}"? Products stay in your catalog and categories are unchanged.`,
                                                )
                                            )
                                                return;
                                            deleteMutation.mutate(c.id);
                                        }}
                                    >
                                        <Trash2 className="size-4" />
                                    </Button>
                                </div>
                            </li>
                        );
                    })}
                </ul>
            )}
        </div>
    );
}
