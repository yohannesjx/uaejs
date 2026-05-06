"use client";

import { useQuery } from "@tanstack/react-query";

import { DataTable } from "@/components/data-table";
import { KpiCard, PageHeader } from "@/components/dashboard-blocks";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/primitives";
import { api } from "@/lib/api-client";
import { formatCurrency } from "@/lib/utils";
import type { FraudSignal, PromotionInsight, ReorderSuggestion } from "@/types/api";
import { createColumnHelper } from "@tanstack/react-table";

const reorderColumn = createColumnHelper<ReorderSuggestion>();
const fraudColumn = createColumnHelper<FraudSignal>();
const promoColumn = createColumnHelper<PromotionInsight>();

export default function DashboardPage() {
  const reorderQuery = useQuery({
    queryKey: ["analytics", "reorders"],
    queryFn: api.getReorders,
  });

  const fraudQuery = useQuery({
    queryKey: ["analytics", "fraud"],
    queryFn: api.getFraudSignals,
  });

  const promotionQuery = useQuery({
    queryKey: ["analytics", "promotions"],
    queryFn: api.getPromotionAnalytics,
  });

  const reorderRows = reorderQuery.data ?? [];
  const fraudRows = fraudQuery.data ?? [];
  const promotionRows = promotionQuery.data?.promotions ?? [];

  return (
    <div className="space-y-8">
      <PageHeader
        title="Executive dashboard"
        description="Sales, promotions, inventory health, and risk signals in one command surface."
      />

      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <KpiCard
          title="Reorder suggestions"
          value={String(reorderRows.length)}
          delta="Live from analytics.reorder"
          data={reorderRows.slice(0, 8).map((row, index) => ({
            value: Number(row.suggested_order_qty) + index,
          }))}
        />
        <KpiCard
          title="Fraud alerts"
          value={String(fraudRows.length)}
          delta="High-return customers flagged"
          tone={fraudRows.length > 0 ? "warning" : "default"}
          data={fraudRows.slice(0, 8).map((row, index) => ({
            value: Number(row.qc_mismatches) + index,
          }))}
        />
        <KpiCard
          title="Promotion insights"
          value={String(promotionRows.length)}
          delta="Revenue-lift snapshots"
          tone="success"
          data={promotionRows.slice(0, 8).map((row, index) => ({
            value: Number(row.revenue_lift ?? index + 1),
          }))}
        />
        <KpiCard
          title="Projected reorder spend"
          value={formatCurrency(
            reorderRows.reduce(
              (sum, row) => sum + Number(row.suggested_order_qty || 0),
              0,
            ),
          )}
          delta="Estimated by current suggestions"
          tone="warning"
          data={reorderRows.slice(0, 8).map((row) => ({
            value: Number(row.current_stock),
          }))}
        />
      </div>

      <div className="grid gap-6 xl:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>Priority replenishment queue</CardTitle>
            <CardDescription>
              Top reorder candidates from the analytics service.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <DataTable
              data={reorderRows}
              columns={[
                reorderColumn.accessor("sku", { header: "SKU" }),
                reorderColumn.accessor("product_name", { header: "Product" }),
                reorderColumn.accessor("current_stock", { header: "Stock" }),
                reorderColumn.accessor("suggested_order_qty", {
                  header: "Suggested qty",
                }),
                reorderColumn.accessor("priority", { header: "Priority" }),
              ]}
              searchPlaceholder="Filter reorder candidates..."
            />
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Return and QC risk</CardTitle>
            <CardDescription>
              Customers and accounts showing elevated mismatch or return behavior.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <DataTable
              data={fraudRows}
              columns={[
                fraudColumn.accessor("customer_email", { header: "Customer" }),
                fraudColumn.accessor("risk_level", { header: "Risk" }),
                fraudColumn.accessor("total_returns", { header: "Returns" }),
                fraudColumn.accessor("qc_mismatches", {
                  header: "QC mismatches",
                }),
                fraudColumn.accessor("reason", { header: "Reason" }),
              ]}
              searchPlaceholder="Filter risk signals..."
            />
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Promotion performance</CardTitle>
          <CardDescription>
            Mixed-case backend payload normalized visually for the dashboard.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            data={promotionRows}
            columns={[
              promoColumn.accessor("SKU", { header: "SKU" }),
              promoColumn.accessor("Channel", { header: "Channel" }),
              promoColumn.accessor("PromoPrice", {
                header: "Promo price",
                cell: ({ getValue }) => formatCurrency(getValue()),
              }),
              promoColumn.accessor("StandardPrice", {
                header: "Standard",
                cell: ({ getValue }) => formatCurrency(getValue()),
              }),
              promoColumn.accessor("revenue_lift", { header: "Revenue lift" }),
              promoColumn.accessor("verdict", { header: "Verdict" }),
            ]}
            searchPlaceholder="Filter promotion insights..."
          />
        </CardContent>
      </Card>
    </div>
  );
}
