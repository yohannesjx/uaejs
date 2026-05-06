"use client";

import { useState } from "react";
import { X } from "lucide-react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/primitives";
import type { ProductStatus } from "@/types/api";

interface ProductStatusPanelProps {
    status: ProductStatus;
    onStatusChange: (status: ProductStatus) => void;
    tags: string[];
    onTagsChange: (tags: string[]) => void;
}

const STATUS_OPTIONS: { value: ProductStatus; label: string; color: string }[] = [
    { value: "active", label: "Active", color: "text-emerald-600 dark:text-emerald-400" },
    { value: "draft", label: "Draft", color: "text-amber-600 dark:text-amber-400" },
    { value: "archived", label: "Archived", color: "text-[var(--muted-foreground)]" },
];

export function ProductStatusPanel({
    status,
    onStatusChange,
    tags,
    onTagsChange,
}: ProductStatusPanelProps) {
    const [tagInput, setTagInput] = useState("");

    const addTag = (raw: string) => {
        const tag = raw.trim().toLowerCase().replace(/\s+/g, "-");
        if (tag && !tags.includes(tag)) {
            onTagsChange([...tags, tag]);
        }
        setTagInput("");
    };

    const removeTag = (tag: string) => {
        onTagsChange(tags.filter((t) => t !== tag));
    };

    const currentStatus = STATUS_OPTIONS.find((s) => s.value === status)!;

    return (
        <div className="space-y-4">
            {/* Status */}
            <Card>
                <CardHeader>
                    <CardTitle>Status</CardTitle>
                </CardHeader>
                <CardContent>
                    <div className="space-y-2">
                        {STATUS_OPTIONS.map((opt) => (
                            <label
                                key={opt.value}
                                className="flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2 transition hover:bg-[var(--muted)]"
                            >
                                <input
                                    type="radio"
                                    name="product-status"
                                    value={opt.value}
                                    checked={status === opt.value}
                                    onChange={() => onStatusChange(opt.value)}
                                    className="accent-[var(--primary)]"
                                />
                                <div>
                                    <span className={`text-sm font-medium ${opt.color}`}>
                                        {opt.label}
                                    </span>
                                    {opt.value === "draft" && (
                                        <p className="text-xs text-[var(--muted-foreground)]">
                                            Not visible to customers
                                        </p>
                                    )}
                                    {opt.value === "active" && (
                                        <p className="text-xs text-[var(--muted-foreground)]">
                                            Visible and purchasable
                                        </p>
                                    )}
                                    {opt.value === "archived" && (
                                        <p className="text-xs text-[var(--muted-foreground)]">
                                            Hidden from all channels
                                        </p>
                                    )}
                                </div>
                            </label>
                        ))}
                    </div>

                    <div className={`mt-3 rounded-lg px-3 py-2 text-xs font-medium ${status === "active"
                            ? "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400"
                            : status === "draft"
                                ? "bg-amber-500/10 text-amber-600 dark:text-amber-400"
                                : "bg-[var(--muted)] text-[var(--muted-foreground)]"
                        }`}>
                        Current: {currentStatus.label}
                    </div>
                </CardContent>
            </Card>

            {/* Tags */}
            <Card>
                <CardHeader>
                    <CardTitle>Tags</CardTitle>
                </CardHeader>
                <CardContent className="space-y-3">
                    <input
                        type="text"
                        value={tagInput}
                        onChange={(e) => setTagInput(e.target.value)}
                        onKeyDown={(e) => {
                            if (e.key === "Enter" || e.key === ",") {
                                e.preventDefault();
                                addTag(tagInput);
                            }
                        }}
                        onBlur={() => tagInput.trim() && addTag(tagInput)}
                        placeholder="summer, sale, new-arrival…"
                        className="flex h-10 w-full rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm outline-none transition placeholder:text-[var(--muted-foreground)] focus:border-[var(--ring)] focus:ring-2 focus:ring-[var(--ring)]/30"
                    />
                    <p className="text-xs text-[var(--muted-foreground)]">
                        Press Enter or comma to add
                    </p>
                    {tags.length > 0 && (
                        <div className="flex flex-wrap gap-1.5">
                            {tags.map((tag) => (
                                <span
                                    key={tag}
                                    className="inline-flex items-center gap-1 rounded-full bg-[var(--muted)] px-2.5 py-1 text-xs font-medium"
                                >
                                    {tag}
                                    <button
                                        type="button"
                                        onClick={() => removeTag(tag)}
                                        className="rounded-full p-0.5 hover:bg-[var(--border)]"
                                    >
                                        <X className="size-2.5" />
                                    </button>
                                </span>
                            ))}
                        </div>
                    )}
                </CardContent>
            </Card>
        </div>
    );
}
