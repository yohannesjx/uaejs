"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery } from "@tanstack/react-query";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { PageHeader } from "@/components/dashboard-blocks";
import { Button, Card, CardContent, CardDescription, CardHeader, CardTitle, Input, Label } from "@/components/ui/primitives";
import { api } from "@/lib/api-client";
import { useAuth } from "@/providers/auth-provider";

const settingsSchema = z.object({
  currency: z.string().min(3),
  vat_rate: z.string().min(1),
  timezone: z.string().min(1),
  color_mode: z.string().min(1),
});

export default function SettingsPage() {
  const { selectedTenantId, setSelectedTenantId } = useAuth();

  const tenantsQuery = useQuery({
    queryKey: ["tenants"],
    queryFn: api.listTenants,
  });

  const settingsQuery = useQuery({
    queryKey: ["tenant-settings", selectedTenantId],
    queryFn: () => api.getTenantSettings(selectedTenantId!),
    enabled: Boolean(selectedTenantId),
  });

  const form = useForm<z.infer<typeof settingsSchema>>({
    resolver: zodResolver(settingsSchema),
    values: {
      currency: String(settingsQuery.data?.settings.currency ?? "AED"),
      vat_rate: String(settingsQuery.data?.settings.vat_rate ?? "0.05"),
      timezone: String(settingsQuery.data?.settings.timezone ?? "Asia/Dubai"),
      color_mode: String(settingsQuery.data?.settings.color_mode ?? "system"),
    },
  });

  const saveMutation = useMutation({
    mutationFn: (values: z.infer<typeof settingsSchema>) =>
      api.saveTenantSettings(selectedTenantId!, values),
  });

  return (
    <div className="space-y-8">
      <PageHeader
        title="Tenant settings"
        description="Select a tenant, persist dashboard preferences, and store theme defaults in tenant settings."
      />

      <div className="grid gap-6 xl:grid-cols-[0.65fr_1.35fr]">
        <Card>
          <CardHeader>
            <CardTitle>Tenant selector</CardTitle>
            <CardDescription>
              Stored locally and sent through <code>X-Tenant-ID</code> for tenant-aware routes.
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-2">
            {tenantsQuery.data?.map((tenant) => (
              <button
                key={tenant.id}
                type="button"
                onClick={() => setSelectedTenantId(tenant.id)}
                className="flex w-full items-center justify-between rounded-xl border border-[var(--border)] px-3 py-3 text-left text-sm hover:bg-[var(--muted)]"
              >
                <span>
                  <span className="block font-medium">{tenant.name}</span>
                  <span className="text-xs text-[var(--muted-foreground)]">
                    {tenant.domain ?? tenant.id}
                  </span>
                </span>
                <span className="text-xs text-[var(--muted-foreground)]">
                  {selectedTenantId === tenant.id ? "Selected" : "Select"}
                </span>
              </button>
            ))}
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>Operational preferences</CardTitle>
            <CardDescription>
              The backend accepts raw JSON and persists it as tenant settings.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form
              className="grid gap-4 md:grid-cols-2"
              onSubmit={form.handleSubmit((values) => saveMutation.mutate(values))}
            >
              <div className="space-y-2">
                <Label>Currency</Label>
                <Input {...form.register("currency")} />
              </div>
              <div className="space-y-2">
                <Label>VAT rate</Label>
                <Input {...form.register("vat_rate")} />
              </div>
              <div className="space-y-2">
                <Label>Timezone</Label>
                <Input {...form.register("timezone")} />
              </div>
              <div className="space-y-2">
                <Label>Color mode</Label>
                <Input {...form.register("color_mode")} />
              </div>
              <div className="md:col-span-2">
                <Button
                  type="submit"
                  disabled={!selectedTenantId}
                  loading={saveMutation.isPending}
                >
                  Save tenant settings
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
