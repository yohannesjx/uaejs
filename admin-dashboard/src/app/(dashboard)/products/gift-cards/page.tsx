"use client";

import { GiftIcon } from "lucide-react";
import { PageHeader } from "@/components/dashboard-blocks";

export default function GiftCardsPage() {
    return (
        <div className="space-y-6">
            <PageHeader
                title="Gift Cards"
                description="Create and manage gift cards for your store."
            />
            <div className="flex min-h-[360px] flex-col items-center justify-center rounded-2xl border border-dashed border-[var(--border)] bg-[var(--panel)] text-center px-8 py-16">
                <div className="mb-6 flex size-16 items-center justify-center rounded-2xl bg-[var(--muted)]">
                    <GiftIcon className="size-8 text-[var(--muted-foreground)]" />
                </div>
                <h2 className="mb-2 text-lg font-semibold">Gift cards coming soon</h2>
                <p className="max-w-sm text-sm text-[var(--muted-foreground)]">
                    Issue, redeem, and track gift card balances. Integration with the POS and
                    e-commerce channels is planned for a future release.
                </p>
            </div>
        </div>
    );
}
