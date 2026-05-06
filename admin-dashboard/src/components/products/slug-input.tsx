"use client";

import { useState, useEffect } from "react";
import { Lock, Unlock } from "lucide-react";
import { Input } from "@/components/ui/primitives";
import { cn } from "@/lib/utils";

export function toSlug(title: string): string {
    return title
        .toLowerCase()
        .trim()
        .replace(/[^\w\s-]/g, "")
        .replace(/[\s_]+/g, "-")
        .replace(/--+/g, "-");
}

interface SlugInputProps {
    title: string;
    value: string;
    onChange: (slug: string) => void;
    className?: string;
    /** URL segment before the handle, e.g. "/products/" or "/collection/". Default "/products/". */
    permalinkPrefix?: string;
    label?: string;
    helpAuto?: string;
    helpLocked?: string;
}

export function SlugInput({
    title,
    value,
    onChange,
    className,
    permalinkPrefix = "/products/",
    label = "URL handle",
    helpAuto = "Auto-generated from title",
    helpLocked = "Editing manually — click lock to auto-generate",
}: SlugInputProps) {
    const [locked, setLocked] = useState(false);

    useEffect(() => {
        if (!locked) {
            const nextSlug = toSlug(title);
            if (nextSlug !== value) {
                onChange(nextSlug);
            }
        }
    }, [title, locked, value, onChange]);

    return (
        <div className={cn("space-y-1.5", className)}>
            <label className="text-sm font-medium text-[var(--foreground)]">
                {label}
            </label>
            <div className="flex items-center gap-2">
                <span className="shrink-0 text-xs text-[var(--muted-foreground)] sm:text-sm font-mono truncate max-w-[40%]">
                    {permalinkPrefix}
                </span>
                <div className="relative flex-1">
                    <Input
                        value={value}
                        onChange={(e) => {
                            setLocked(true);
                            onChange(toSlug(e.target.value));
                        }}
                        className="pr-9 font-mono text-sm"
                    />
                    <button
                        type="button"
                        title={locked ? "Unlock (auto-generate from title)" : "Lock slug"}
                        onClick={() => setLocked((l) => !l)}
                        className="absolute right-2.5 top-1/2 -translate-y-1/2 text-[var(--muted-foreground)] hover:text-[var(--foreground)]"
                    >
                        {locked ? <Lock className="size-3.5" /> : <Unlock className="size-3.5" />}
                    </button>
                </div>
            </div>
            <p className="text-xs text-[var(--muted-foreground)]">
                {locked ? helpLocked : helpAuto}
            </p>
        </div>
    );
}
