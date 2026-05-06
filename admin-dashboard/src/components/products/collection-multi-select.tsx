"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Check, ChevronDown, Layers, Search, X } from "lucide-react";

import { Badge, Button } from "@/components/ui/primitives";
import { api } from "@/lib/api-client";
import { cn } from "@/lib/utils";

function useDebounce<T>(value: T, delay: number): T {
    const [debouncedValue, setDebouncedValue] = useState<T>(value);
    useEffect(() => {
        const handler = setTimeout(() => setDebouncedValue(value), delay);
        return () => clearTimeout(handler);
    }, [value, delay]);
    return debouncedValue;
}

type CollectionMultiSelectProps = {
    value: string[];
    onChange: (ids: string[]) => void;
};

function filterCollections(
    list: { id: string; title: string; slug: string }[],
    query: string,
) {
    const q = query.trim().toLowerCase();
    if (!q) return [...list].sort((a, b) => a.title.localeCompare(b.title, undefined, { sensitivity: "base" }));

    const tokens = q.split(/\s+/).filter(Boolean);
    const scored = list
        .map((c) => {
            const titleL = c.title.toLowerCase();
            const slugL = c.slug.toLowerCase();
            const hay = `${titleL} ${slugL}`;
            const matches = tokens.every((t) => hay.includes(t));
            if (!matches) return null;
            let score = 0;
            if (titleL.startsWith(q) || slugL.startsWith(q)) score += 100;
            tokens.forEach((t) => {
                if (titleL.startsWith(t)) score += 20;
                else if (slugL.startsWith(t)) score += 10;
                else if (titleL.includes(t)) score += 5;
            });
            return { c, score };
        })
        .filter((x): x is NonNullable<typeof x> => x !== null)
        .sort((a, b) => b.score - a.score || a.c.title.localeCompare(b.c.title, undefined, { sensitivity: "base" }));

    return scored.map((x) => x.c);
}

export function CollectionMultiSelect({ value, onChange }: CollectionMultiSelectProps) {
    const [open, setOpen] = useState(false);
    const [search, setSearch] = useState("");
    const debouncedSearch = useDebounce(search, 200);

    const { data: collections = [], isLoading } = useQuery({
        queryKey: ["collections"],
        queryFn: () => api.listCollections(),
        staleTime: 60_000,
    });

    const selectedSet = useMemo(() => new Set(value), [value]);

    const filtered = useMemo(
        () => filterCollections(collections, debouncedSearch),
        [collections, debouncedSearch],
    );

    const idToTitle = useMemo(() => {
        const m = new Map<string, string>();
        collections.forEach((c) => m.set(c.id, c.title));
        return m;
    }, [collections]);

    const toggle = useCallback(
        (id: string) => {
            const next = new Set(selectedSet);
            if (next.has(id)) next.delete(id);
            else next.add(id);
            onChange(Array.from(next));
        },
        [selectedSet, onChange],
    );

    const remove = useCallback(
        (id: string) => {
            onChange(value.filter((x) => x !== id));
        },
        [value, onChange],
    );

    useEffect(() => {
        if (!open) return;
        const onKey = (e: KeyboardEvent) => {
            if (e.key === "Escape") setOpen(false);
        };
        window.addEventListener("keydown", onKey);
        return () => window.removeEventListener("keydown", onKey);
    }, [open]);

    const summary =
        value.length === 0
            ? "No collections selected"
            : value.length === 1
              ? idToTitle.get(value[0]) ?? "1 collection"
              : `${value.length} collections`;

    return (
        <div className="space-y-2">
            {value.length > 0 ? (
                <div className="flex flex-wrap gap-1.5">
                    {value.map((id) => (
                        <Badge
                            key={id}
                            className="max-w-full gap-1 rounded-lg pl-2 pr-1 font-normal"
                        >
                            <span className="truncate">{idToTitle.get(id) ?? id.slice(0, 8)}</span>
                            <button
                                type="button"
                                className="rounded p-0.5 hover:bg-[var(--muted)]"
                                aria-label={`Remove ${idToTitle.get(id) ?? "collection"}`}
                                onClick={() => remove(id)}
                            >
                                <X className="size-3.5 shrink-0 opacity-70" />
                            </button>
                        </Badge>
                    ))}
                </div>
            ) : null}

            <div className="relative">
                <button
                    type="button"
                    onClick={() => setOpen((o) => !o)}
                    className="flex h-10 w-full items-center justify-between gap-2 rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm transition hover:border-[var(--ring)] focus:outline-none focus:ring-2 focus:ring-[var(--ring)]/30"
                    aria-expanded={open}
                    aria-haspopup="listbox"
                >
                    <span className="flex min-w-0 items-center gap-2">
                        <Layers className="size-4 shrink-0 text-[var(--muted-foreground)]" />
                        <span className={cn("truncate text-left", value.length === 0 && "text-[var(--muted-foreground)]")}>
                            {summary}
                        </span>
                    </span>
                    <ChevronDown className={cn("size-4 shrink-0 text-[var(--muted-foreground)] transition-transform", open && "rotate-180")} />
                </button>

                {open && (
                    <>
                        <div className="fixed inset-0 z-10" onClick={() => setOpen(false)} aria-hidden />
                        <div className="absolute left-0 top-full z-20 mt-1 w-full overflow-hidden rounded-xl border border-[var(--border)] bg-[var(--panel)] shadow-xl">
                            <div className="flex items-center gap-2 border-b border-[var(--border)] px-3 py-2">
                                <Search className="size-3.5 shrink-0 text-[var(--muted-foreground)]" />
                                <input
                                    autoFocus
                                    type="text"
                                    value={search}
                                    onChange={(e) => setSearch(e.target.value)}
                                    placeholder="Search by name or handle…"
                                    className="w-full bg-transparent text-sm outline-none placeholder:text-[var(--muted-foreground)]"
                                    onKeyDown={(e) => {
                                        if (e.key === "Escape") setOpen(false);
                                    }}
                                />
                            </div>
                            <div className="max-h-56 overflow-y-auto py-1" role="listbox">
                                {isLoading && (
                                    <p className="px-3 py-3 text-xs text-[var(--muted-foreground)]">Loading collections…</p>
                                )}
                                {!isLoading && collections.length === 0 && (
                                    <p className="px-3 py-3 text-xs text-[var(--muted-foreground)]">No collections yet. Create one from the sidebar.</p>
                                )}
                                {!isLoading &&
                                    collections.length > 0 &&
                                    filtered.length === 0 &&
                                    debouncedSearch.trim().length > 0 && (
                                        <p className="px-3 py-3 text-xs text-[var(--muted-foreground)]">No matches</p>
                                    )}
                                {!isLoading &&
                                    collections.length > 0 &&
                                    filtered.length === 0 &&
                                    debouncedSearch.trim().length === 0 && (
                                        <p className="px-3 py-3 text-xs text-[var(--muted-foreground)]">Start typing to search</p>
                                    )}
                                {filtered.map((c) => {
                                    const checked = selectedSet.has(c.id);
                                    return (
                                        <button
                                            key={c.id}
                                            type="button"
                                            role="option"
                                            aria-selected={checked}
                                            onClick={() => toggle(c.id)}
                                            className="flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition hover:bg-[var(--muted)]"
                                        >
                                            <span
                                                className={cn(
                                                    "flex size-4 shrink-0 items-center justify-center rounded border border-[var(--border)]",
                                                    checked && "border-[var(--primary)] bg-[var(--primary)] text-white",
                                                )}
                                            >
                                                {checked ? <Check className="size-3" /> : null}
                                            </span>
                                            <span className="min-w-0 flex-1 truncate font-medium">{c.title}</span>
                                            <span className="shrink-0 text-xs text-[var(--muted-foreground)]">/{c.slug}</span>
                                        </button>
                                    );
                                })}
                            </div>
                            {value.length > 0 ? (
                                <div className="flex justify-end border-t border-[var(--border)] px-2 py-1.5">
                                    <Button type="button" variant="ghost" size="sm" className="text-xs h-8" onClick={() => onChange([])}>
                                        Clear all
                                    </Button>
                                </div>
                            ) : null}
                        </div>
                    </>
                )}
            </div>
        </div>
    );
}
