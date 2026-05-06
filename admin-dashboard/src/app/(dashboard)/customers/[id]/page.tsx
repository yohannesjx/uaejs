"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { createColumnHelper } from "@tanstack/react-table";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { DataTable } from "@/components/data-table";
import { PageHeader } from "@/components/dashboard-blocks";
import { Badge, Button, Card, CardContent, CardDescription, CardHeader, CardTitle, Input, Label } from "@/components/ui/primitives";
import { api } from "@/lib/api-client";
import { formatCurrency, formatDate } from "@/lib/utils";
import type { LoyaltyTransaction } from "@/types/api";

const historyColumn = createColumnHelper<LoyaltyTransaction>();

const awardSchema = z.object({
  order_id: z.string().uuid(),
  total_aed: z.string().min(1),
});

const redeemSchema = z.object({
  points_to_redeem: z.string().min(1),
});

export default function CustomerDetailPage({
  params,
}: {
  params: { id: string };
}) {
  const profileQuery = useQuery({
    queryKey: ["customer", params.id],
    queryFn: () => api.getCustomer(params.id),
  });

  const historyQuery = useQuery({
    queryKey: ["customer-history", params.id],
    queryFn: () => api.getCustomerPointsHistory(params.id, 100),
  });

  const awardForm = useForm<z.infer<typeof awardSchema>>({
    resolver: zodResolver(awardSchema),
    defaultValues: { order_id: "", total_aed: "" },
  });

  const redeemForm = useForm<z.infer<typeof redeemSchema>>({
    resolver: zodResolver(redeemSchema),
    defaultValues: { points_to_redeem: "100" },
  });

  const awardMutation = useMutation({
    mutationFn: (values: z.infer<typeof awardSchema>) =>
      api.addCustomerPoints(params.id, values),
  });

  const redeemMutation = useMutation({
    mutationFn: (values: { points_to_redeem: number }) =>
      api.redeemCustomerPoints(params.id, values.points_to_redeem),
  });

  const customer = profileQuery.data?.customer;
  const loyalty = profileQuery.data?.loyalty_account;

  return (
    <div className="space-y-8">
      <PageHeader
        title={customer?.full_name ?? "Customer profile"}
        description="Loyalty account status, redemption actions, and immutable points ledger."
      />

      <div className="grid gap-6 xl:grid-cols-[1.15fr_0.85fr]">
        <Card>
          <CardHeader>
            <CardTitle>Profile</CardTitle>
            <CardDescription>Tenant-scoped customer account details.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-4 md:grid-cols-2">
            <div>
              <p className="text-xs uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
                Email
              </p>
              <p className="mt-1 font-medium">{customer?.email ?? "—"}</p>
            </div>
            <div>
              <p className="text-xs uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
                Tier
              </p>
              <div className="mt-1">
                <Badge>{customer?.loyalty_tier ?? "bronze"}</Badge>
              </div>
            </div>
            <div>
              <p className="text-xs uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
                Points balance
              </p>
              <p className="mt-1 text-2xl font-semibold">
                {loyalty?.points_balance ?? 0}
              </p>
            </div>
            <div>
              <p className="text-xs uppercase tracking-[0.18em] text-[var(--muted-foreground)]">
                Lifetime points
              </p>
              <p className="mt-1 text-2xl font-semibold">
                {loyalty?.lifetime_points ?? 0}
              </p>
            </div>
          </CardContent>
        </Card>

        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Award points</CardTitle>
              <CardDescription>1 point per AED spent.</CardDescription>
            </CardHeader>
            <CardContent>
              <form
                className="space-y-3"
                onSubmit={awardForm.handleSubmit((values) =>
                  awardMutation.mutate(values),
                )}
              >
                <div className="space-y-2">
                  <Label>Order ID</Label>
                  <Input {...awardForm.register("order_id")} placeholder="UUID" />
                </div>
                <div className="space-y-2">
                  <Label>Total AED</Label>
                  <Input {...awardForm.register("total_aed")} placeholder="250.00" />
                </div>
                <Button type="submit" className="w-full" loading={awardMutation.isPending}>
                  Add points
                </Button>
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Redeem points</CardTitle>
              <CardDescription>100 points = 1 AED discount.</CardDescription>
            </CardHeader>
            <CardContent>
              <form
                className="space-y-3"
                onSubmit={redeemForm.handleSubmit((values) =>
                  redeemMutation.mutate({
                    points_to_redeem: Number(values.points_to_redeem),
                  }),
                )}
              >
                <div className="space-y-2">
                  <Label>Points to redeem</Label>
                  <Input type="number" {...redeemForm.register("points_to_redeem")} />
                </div>
                <Button type="submit" className="w-full" loading={redeemMutation.isPending}>
                  Redeem now
                </Button>
                {redeemMutation.data ? (
                  <p className="text-xs text-[var(--muted-foreground)]">
                    Discount generated:{" "}
                    {formatCurrency(redeemMutation.data.discount_aed)}
                  </p>
                ) : null}
              </form>
            </CardContent>
          </Card>
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Loyalty ledger</CardTitle>
          <CardDescription>
            Immutable transaction history for auditing earned and redeemed points.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DataTable
            data={historyQuery.data ?? []}
            columns={[
              historyColumn.accessor("tx_type", { header: "Type" }),
              historyColumn.accessor("points", { header: "Points" }),
              historyColumn.accessor("balance_before", { header: "Before" }),
              historyColumn.accessor("balance_after", { header: "After" }),
              historyColumn.accessor("note", { header: "Note" }),
              historyColumn.accessor("created_at", {
                header: "Created",
                cell: ({ getValue }) => formatDate(getValue()),
              }),
            ]}
            searchPlaceholder="Filter point history..."
          />
        </CardContent>
      </Card>
    </div>
  );
}
