"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, ChevronDown, Folder, Search } from "lucide-react";
import { api } from "@/lib/api-client";
import { cn } from "@/lib/utils";
import type { ProductCategory } from "@/types/api";

interface CategorySelectorProps {
    value?: string | null;
    onChange: (id: string | null, category?: ProductCategory) => void;
    placeholder?: string;
}

export function CategorySelector({
    value,
    onChange,
    placeholder = "Select category",
}: CategorySelectorProps) {
    const [open, setOpen] = useState(false);
    const [search, setSearch] = useState("");

    const { data: categories = [] } = useQuery({
        queryKey: ["categories"],
        queryFn: () => api.listCategories(),
        staleTime: 60_000,
    });

    const selected = categories.find((c) => c.id === value);
    const filtered = categories.filter((c) =>
        c.title.toLowerCase().includes(search.toLowerCase()),
    );

    return (
        <div className="relative">
            <button
                type="button"
                onClick={() => setOpen((o) => !o)}
                className="flex h-10 w-full items-center justify-between rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm transition hover:border-[var(--ring)] focus:outline-none focus:ring-2 focus:ring-[var(--ring)]/30"
            >
                <span className={selected ? "text-[var(--foreground)]" : "text-[var(--muted-foreground)]"}>
                    {selected ? selected.title : placeholder}
                </span>
                <ChevronDown className="size-4 text-[var(--muted-foreground)]" />
            </button>

            {open && (
                <>
                    <div
                        className="fixed inset-0 z-10"
                        onClick={() => setOpen(false)}
                    />
                    <div className="absolute left-0 top-full z-20 mt-1 w-full min-w-[220px] overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--panel)] shadow-xl">
                        {/* Search */}
                        <div className="flex items-center gap-2 border-b border-[var(--border)] px-3 py-2">
                            <Search className="size-3.5 shrink-0 text-[var(--muted-foreground)]" />
                            <input
                                autoFocus
                                type="text"
                                value={search}
                                onChange={(e) => setSearch(e.target.value)}
                                placeholder="Search categories…"
                                className="w-full bg-transparent text-sm outline-none"
                            />
                        </div>

                        <div className="max-h-56 overflow-y-auto py-1">
                            {/* None option */}
                            <button
                                type="button"
                                onClick={() => { onChange(null); setOpen(false); }}
                                className={cn(
                                    "flex w-full items-center gap-2 px-3 py-2 text-sm transition hover:bg-[var(--muted)]",
                                    !value && "font-medium",
                                )}
                            >
                                <Check className={cn("size-3.5", !value ? "opacity-100" : "opacity-0")} />
                                <span className="text-[var(--muted-foreground)]">No category</span>
                            </button>

                            {filtered.length === 0 ? (
                                <p className="px-3 py-4 text-center text-xs text-[var(--muted-foreground)]">
                                    {categories.length === 0
                                        ? "No categories yet. Create one under Products → Categories."
                                        : "No results"}
                                </p>
                            ) : (
                                filtered.map((cat) => (
                                    <button
                                        key={cat.id}
                                        type="button"
                                        onClick={() => { onChange(cat.id, cat); setOpen(false); }}
                                        className={cn(
                                            "flex w-full items-center gap-2 px-3 py-2 text-sm transition hover:bg-[var(--muted)]",
                                            cat.id === value && "font-medium",
                                        )}
                                    >
                                        <Check
                                            className={cn("size-3.5", cat.id === value ? "opacity-100" : "opacity-0")}
                                        />
                                        <Folder className="size-3.5 text-[var(--muted-foreground)]" />
                                        {cat.title}
                                    </button>
                                ))
                            )}
                        </div>
                    </div>
                </>
            )}
        </div>
    );
}
