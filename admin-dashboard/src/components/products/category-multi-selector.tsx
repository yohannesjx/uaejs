"use client";

import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Search } from "lucide-react";
import { api } from "@/lib/api-client";
import { cn } from "@/lib/utils";

interface CategoryMultiSelectorProps {
    value: string[];
    onChange: (ids: string[]) => void;
    /** Optional hint under the list */
    helperText?: string;
}

export function CategoryMultiSelector({
    value,
    onChange,
    helperText = "Select one or more categories.",
}: CategoryMultiSelectorProps) {
    const [search, setSearch] = useState("");

    const { data: categories = [] } = useQuery({
        queryKey: ["categories"],
        queryFn: () => api.listCategories(),
        staleTime: 60_000,
    });

    const selected = useMemo(() => new Set(value), [value]);
    const filtered = useMemo(
        () =>
            categories.filter((c) =>
                c.title.toLowerCase().includes(search.toLowerCase()),
            ),
        [categories, search],
    );

    const toggle = (id: string) => {
        const next = new Set(value);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        onChange(Array.from(next));
    };

    return (
        <div className="space-y-2">
            <div className="rounded-xl border border-[var(--border)] bg-[var(--panel)] p-3">
                <div className="flex items-center gap-2 border-b border-[var(--border)] pb-2 mb-2">
                    <Search className="size-3.5 shrink-0 text-[var(--muted-foreground)]" />
                    <input
                        type="text"
                        value={search}
                        onChange={(e) => setSearch(e.target.value)}
                        placeholder="Search categories…"
                        className="w-full bg-transparent text-sm outline-none"
                    />
                </div>
                <div className="max-h-52 space-y-1 overflow-y-auto pr-1">
                    {categories.length === 0 ? (
                        <p className="py-3 text-center text-xs text-[var(--muted-foreground)]">
                            No categories yet. Create one under Products → Categories.
                        </p>
                    ) : filtered.length === 0 ? (
                        <p className="py-3 text-center text-xs text-[var(--muted-foreground)]">No results</p>
                    ) : (
                        filtered.map((cat) => {
                            const checked = selected.has(cat.id);
                            return (
                                <label
                                    key={cat.id}
                                    className={cn(
                                        "flex cursor-pointer items-center gap-3 rounded-lg border px-3 py-2.5 text-sm transition",
                                        checked
                                            ? "border-[var(--ring)] bg-[var(--muted)]/60"
                                            : "border-transparent hover:bg-[var(--muted)]/40",
                                    )}
                                >
                                    <input
                                        type="checkbox"
                                        className="size-4 shrink-0 rounded border-[var(--border)] accent-black"
                                        checked={checked}
                                        onChange={() => toggle(cat.id)}
                                    />
                                    <span className="min-w-0 flex-1 font-medium leading-snug">{cat.title}</span>
                                </label>
                            );
                        })
                    )}
                </div>
            </div>
            {helperText ? (
                <p className="text-xs text-[var(--muted-foreground)]">{helperText}</p>
            ) : null}
        </div>
    );
}
