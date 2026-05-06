"use client";

import { Area, AreaChart, ResponsiveContainer, Tooltip } from "recharts";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/primitives";
import { cn } from "@/lib/utils";

export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description: string;
  actions?: React.ReactNode;
}) {
  return (
    <div className="mb-6 flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
      <div>
        <h1 className="text-2xl font-semibold tracking-tight">{title}</h1>
        <p className="mt-1 text-sm text-[var(--muted-foreground)]">{description}</p>
      </div>
      {actions ? <div className="flex flex-wrap gap-2">{actions}</div> : null}
    </div>
  );
}

export function KpiCard({
  title,
  value,
  delta,
  data,
  tone = "default",
}: {
  title: string;
  value: string;
  delta?: string;
  data?: Array<{ value: number }>;
  tone?: "default" | "success" | "warning" | "danger";
}) {
  return (
    <Card>
      <CardHeader>
        <CardDescription>{title}</CardDescription>
        <CardTitle className="mt-2 text-2xl">{value}</CardTitle>
      </CardHeader>
      <CardContent className="pt-3">
        {delta ? (
          <p
            className={cn(
              "text-sm",
              tone === "success" && "text-emerald-600 dark:text-emerald-400",
              tone === "warning" && "text-amber-600 dark:text-amber-300",
              tone === "danger" && "text-rose-600 dark:text-rose-300",
              tone === "default" && "text-[var(--muted-foreground)]",
            )}
          >
            {delta}
          </p>
        ) : null}
        {data?.length ? (
          <div className="mt-3 h-16">
            <ResponsiveContainer width="100%" height="100%">
              <AreaChart data={data}>
                <Tooltip />
                <Area
                  type="monotone"
                  dataKey="value"
                  stroke="var(--chart-1)"
                  fill="var(--chart-1)"
                  fillOpacity={0.15}
                  strokeWidth={2}
                />
              </AreaChart>
            </ResponsiveContainer>
          </div>
        ) : null}
      </CardContent>
    </Card>
  );
}

export function EmptyFeature({
  title,
  description,
  action,
}: {
  title: string;
  description: string;
  action?: React.ReactNode;
}) {
  return (
    <Card className="border-dashed">
      <CardContent className="flex min-h-52 flex-col items-center justify-center text-center">
        <h3 className="text-lg font-semibold">{title}</h3>
        <p className="mt-2 max-w-lg text-sm text-[var(--muted-foreground)]">
          {description}
        </p>
        {action ? <div className="mt-4">{action}</div> : null}
      </CardContent>
    </Card>
  );
}
