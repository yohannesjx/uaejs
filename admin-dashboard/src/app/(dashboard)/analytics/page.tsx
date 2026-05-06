"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import {
  CartesianGrid,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { PageHeader } from "@/components/dashboard-blocks";
import {
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  Input,
  Label,
} from "@/components/ui/primitives";
import { api } from "@/lib/api-client";

const forecastSchema = z.object({
  sku: z.string().min(1),
  channel: z.string().optional(),
});

export default function AnalyticsPage() {
  const forecastForm = useForm<z.infer<typeof forecastSchema>>({
    resolver: zodResolver(forecastSchema),
    defaultValues: { sku: "", channel: "" },
  });

  const forecastMutation = useMutation({
    mutationFn: (values: z.infer<typeof forecastSchema>) =>
      api.getForecast(values.sku, values.channel),
  });

  const promotionsQuery = useQuery({
    queryKey: ["analytics-promotions"],
    queryFn: api.getPromotionAnalytics,
  });

  const chartData = (promotionsQuery.data?.promotions ?? [])
    .slice(0, 8)
    .map((item, index) => ({
      name: item.SKU ?? `Promo ${index + 1}`,
      revenueLift: Number(item.revenue_lift ?? 0),
      discountDepth: Number(item.discount_depth ?? 0),
    }));

  return (
    <div className="space-y-8">
      <PageHeader
        title="Analytics"
        description="Demand forecast lookup, promotion lift, and operational planning."
      />

      <div className="grid gap-6 xl:grid-cols-[0.85fr_1.15fr]">
        <Card>
          <CardHeader>
            <CardTitle>Forecast lookup</CardTitle>
            <CardDescription>
              Query <code>/admin/analytics/forecast</code> by SKU.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form
              className="space-y-3"
              onSubmit={forecastForm.handleSubmit((values) =>
                forecastMutation.mutate(values),
              )}
            >
              <div className="space-y-2">
                <Label>SKU</Label>
                <Input {...forecastForm.register("sku")} placeholder="SKU-001" />
              </div>
              <div className="space-y-2">
                <Label>Channel (optional)</Label>
                <Input
                  {...forecastForm.register("channel")}
                  placeholder="ecommerce"
                />
              </div>
              <Button type="submit" className="w-full" loading={forecastMutation.isPending}>
                Run forecast
              </Button>
            </form>

            {forecastMutation.data ? (
              <div className="mt-5 grid gap-3 rounded-2xl bg-[var(--muted)] p-4 text-sm">
                <div className="flex justify-between gap-4">
                  <span>Current stock</span>
                  <strong>{forecastMutation.data.current_stock}</strong>
                </div>
                <div className="flex justify-between gap-4">
                  <span>Weekly forecast</span>
                  <strong>{forecastMutation.data.weekly_forecast_units}</strong>
                </div>
                <div className="flex justify-between gap-4">
                  <span>Days of stock left</span>
                  <strong>{forecastMutation.data.days_of_stock_left}</strong>
                </div>
                <div className="flex justify-between gap-4">
                  <span>Confidence</span>
                  <strong>{forecastMutation.data.confidence}</strong>
                </div>
              </div>
            ) : null}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Promotion lift</CardTitle>
            <CardDescription>
              Quick visual over the promotion analytics endpoint.
            </CardDescription>
          </CardHeader>
          <CardContent className="h-[360px]">
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={chartData}>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border)" />
                <XAxis dataKey="name" hide />
                <YAxis />
                <Tooltip />
                <Line
                  type="monotone"
                  dataKey="revenueLift"
                  stroke="var(--chart-1)"
                  strokeWidth={2}
                />
                <Line
                  type="monotone"
                  dataKey="discountDepth"
                  stroke="#ec4899"
                  strokeWidth={2}
                />
              </LineChart>
            </ResponsiveContainer>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
