"use client";

import { ShoppingCart } from "lucide-react";
import Link from "next/link";
import { Button } from "@/components/ui/primitives";
import { PageHeader } from "@/components/dashboard-blocks";

export default function PurchaseOrdersPage() {
    return (
        <div className="space-y-6">
            <PageHeader
                title="Purchase Orders"
                description="Manage incoming goods from suppliers."
            />
            <div className="flex min-h-[360px] flex-col items-center justify-center rounded-2xl border border-dashed border-[var(--border)] bg-[var(--panel)] text-center px-8 py-16">
                <div className="mb-6 flex size-16 items-center justify-center rounded-2xl bg-[var(--muted)]">
                    <ShoppingCart className="size-8 text-[var(--muted-foreground)]" />
                </div>
                <h2 className="mb-2 text-lg font-semibold">Purchase orders coming soon</h2>
                <p className="mb-8 max-w-sm text-sm text-[var(--muted-foreground)]">
                    Full purchase order management with landed cost tracking, supplier integration,
                    and FIFO batch assignment is on the roadmap.
                </p>
                <Button variant="outline" asChild>
                    <Link href="/suppliers">View suppliers</Link>
                </Button>
            </div>
        </div>
    );
}
