"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { Plus, Folder, Zap } from "lucide-react";

import { Button, Badge } from "@/components/ui/primitives";
import { api } from "@/lib/api-client";

export default function CategoriesPage() {
    const { data: categories = [], isLoading } = useQuery({
        queryKey: ["categories"],
        queryFn: () => api.listCategories(),
        staleTime: 60_000,
        refetchOnWindowFocus: false,
        refetchOnReconnect: false,
    });

    return (
        <div className="space-y-6">
            <div className="flex items-center justify-between">
                <div>
                    <h1 className="text-xl font-semibold">Categories</h1>
                    <p className="text-sm text-[var(--muted-foreground)]">
                        Organise products into manual or smart collections.
                    </p>
                </div>
                <Button asChild>
                    <Link href="/products/categories/new">
                        <Plus className="size-4" />
                        Create category
                    </Link>
                </Button>
            </div>

            {isLoading ? (
                <div className="space-y-2">
                    {[...Array(4)].map((_, i) => (
                        <div key={i} className="h-16 animate-pulse rounded-xl bg-[var(--muted)]" />
                    ))}
                </div>
            ) : categories.length === 0 ? (
                <div className="flex min-h-[360px] flex-col items-center justify-center rounded-2xl border border-dashed border-[var(--border)] bg-[var(--panel)] text-center px-8 py-16">
                    <div className="mb-6 flex size-16 items-center justify-center rounded-2xl bg-[var(--muted)]">
                        <Folder className="size-8 text-[var(--muted-foreground)]" />
                    </div>
                    <h2 className="mb-2 text-lg font-semibold">No categories yet</h2>
                    <p className="max-w-sm text-sm text-[var(--muted-foreground)]">
                        Categories group your products into collections. Create manual collections by hand-picking
                        products, or smart collections that update automatically by rules.
                    </p>
                </div>
            ) : (
                <div className="space-y-2">
                    {categories.map((cat) => (
                        <Link
                            key={cat.id}
                            href={`/products/categories/${cat.id}/edit`}
                            className="flex items-center justify-between rounded-xl border border-[var(--border)] bg-[var(--panel)] px-4 py-3 transition hover:border-[var(--ring)] hover:bg-[var(--muted)]/30"
                        >
                            <div className="flex items-center gap-3">
                                {cat.type === "smart" ? (
                                    <Zap className="size-4 text-amber-500" />
                                ) : (
                                    <Folder className="size-4 text-[var(--muted-foreground)]" />
                                )}
                                <div>
                                    <p className="font-medium">{cat.title}</p>
                                    <p className="text-xs text-[var(--muted-foreground)]">
                                        {cat.product_count ?? 0} products
                                    </p>
                                </div>
                            </div>
                            <div className="flex items-center gap-3">
                                <Badge tone={cat.type === "smart" ? "warning" : "default"}>
                                    {cat.type}
                                </Badge>
                            </div>
                        </Link>
                    ))}
                </div>
            )}
        </div>
    );
}
