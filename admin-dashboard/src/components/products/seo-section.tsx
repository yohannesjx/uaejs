"use client";

import { cn } from "@/lib/utils";

interface SeoSectionProps {
    title: string;
    description: string;
    urlHandle: string;
    onTitleChange: (v: string) => void;
    onDescriptionChange: (v: string) => void;
}

const TITLE_MAX = 70;
const DESC_MAX = 160;

export function SeoSection({
    title,
    description,
    urlHandle,
    onTitleChange,
    onDescriptionChange,
}: SeoSectionProps) {
    return (
        <div className="rounded-xl border border-[var(--border)] bg-[var(--panel)] p-5 space-y-5">
            <h3 className="font-semibold">Search engine listing</h3>

            {/* Google preview */}
            <div className="rounded-xl border border-[var(--border)] bg-[var(--background)] p-4 space-y-0.5">
                <p className="truncate text-base font-medium text-blue-600 dark:text-blue-400">
                    {title || "Page title"}
                </p>
                <p className="truncate text-xs text-emerald-700 dark:text-emerald-400">
                    https://yourstore.ae/products/{urlHandle || "product-url"}
                </p>
                <p className="line-clamp-2 text-xs text-[var(--muted-foreground)]">
                    {description || "Add a meta description to help customers find your product in search results."}
                </p>
            </div>

            {/* Page title */}
            <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                    <label className="text-sm font-medium">Page title</label>
                    <span
                        className={cn(
                            "text-xs",
                            title.length > TITLE_MAX
                                ? "text-red-500"
                                : "text-[var(--muted-foreground)]",
                        )}
                    >
                        {title.length} / {TITLE_MAX}
                    </span>
                </div>
                <input
                    type="text"
                    maxLength={TITLE_MAX + 10}
                    value={title}
                    onChange={(e) => onTitleChange(e.target.value)}
                    placeholder="Descriptive title for search engines"
                    className="flex h-10 w-full rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm outline-none transition placeholder:text-[var(--muted-foreground)] focus:border-[var(--ring)] focus:ring-2 focus:ring-[var(--ring)]/30"
                />
            </div>

            {/* Meta description */}
            <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                    <label className="text-sm font-medium">Meta description</label>
                    <span
                        className={cn(
                            "text-xs",
                            description.length > DESC_MAX
                                ? "text-red-500"
                                : "text-[var(--muted-foreground)]",
                        )}
                    >
                        {description.length} / {DESC_MAX}
                    </span>
                </div>
                <textarea
                    maxLength={DESC_MAX + 10}
                    value={description}
                    onChange={(e) => onDescriptionChange(e.target.value)}
                    rows={3}
                    placeholder="2–3 sentence summary of your product"
                    className="flex min-h-[80px] w-full rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm outline-none transition placeholder:text-[var(--muted-foreground)] focus:border-[var(--ring)] focus:ring-2 focus:ring-[var(--ring)]/30"
                />
            </div>
        </div>
    );
}
