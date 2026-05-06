"use client";

import { createColumnHelper } from "@tanstack/react-table";
import { useQuery } from "@tanstack/react-query";
import { useCallback, useState } from "react";
import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useForm, useWatch } from "react-hook-form";
import { toast } from "sonner";
import { z } from "zod";

import { ServerDataTable } from "@/components/server-data-table";
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
  Textarea,
} from "@/components/ui/primitives";
import { api } from "@/lib/api-client";
import type { CustomerListItem } from "@/types/api";

const createCustomerSchema = z.object({
  email: z.string().email(),
  full_name: z.string().min(2),
  phone: z.string().optional(),
  notes: z.string().optional(),
});

const lookupSchema = z.object({
  customerId: z.string().uuid("Enter a valid customer UUID"),
});

type CreateCustomerValues = z.infer<typeof createCustomerSchema>;
type LookupValues = z.infer<typeof lookupSchema>;

const col = createColumnHelper<CustomerListItem>();

const customerColumns = [
  col.accessor("email", { header: "Email", cell: (c) => c.getValue() }),
  col.accessor("full_name", { header: "Name", cell: (c) => c.getValue() }),
  col.accessor("loyalty_tier", {
    header: "Tier",
    cell: (c) => c.getValue(),
  }),
  col.accessor("points_balance", {
    header: "Points",
    cell: (c) => c.getValue(),
  }),
  col.accessor("is_active", {
    header: "Active",
    cell: (c) => (c.getValue() ? "Yes" : "No"),
  }),
  col.display({
    id: "actions",
    header: "",
    cell: ({ row }) => (
      <Link
        href={`/customers/${row.original.id}`}
        className="text-sm text-[var(--primary)] hover:underline"
      >
        Profile
      </Link>
    ),
  }),
];

export default function CustomersPage() {
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const pageSize = 25;

  const { data, isLoading } = useQuery({
    queryKey: ["customers", page, pageSize, search],
    queryFn: () =>
      api.listCustomers({
        page,
        page_size: pageSize,
        search: search || undefined,
      }),
  });

  const createForm = useForm<CreateCustomerValues>({
    resolver: zodResolver(createCustomerSchema),
    defaultValues: {
      email: "",
      full_name: "",
      phone: "",
      notes: "",
    },
  });

  const lookupForm = useForm<LookupValues>({
    resolver: zodResolver(lookupSchema),
    defaultValues: {
      customerId: "",
    },
  });

  const customerId = useWatch({
    control: lookupForm.control,
    name: "customerId",
  });

  const queryClient = useQueryClient();
  const createMutation = useMutation({
    mutationFn: api.createCustomer,
    onSuccess: () => {
      createForm.reset();
      queryClient.invalidateQueries({ queryKey: ["customers"] });
      toast.success("Customer created");
    },
    onError: (err) => toast.error(err instanceof Error ? err.message : "Failed to create customer"),
  });

  const handleSearchChange = useCallback((s: string) => {
    setSearch(s);
    setPage(1);
  }, []);

  return (
    <div className="space-y-8">
      <PageHeader
        title="Customers & loyalty"
        description="Create customer profiles, manage loyalty, and view points."
      />

      <div className="grid gap-6 xl:grid-cols-[0.5fr_1.5fr]">
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle>Register customer</CardTitle>
              <CardDescription>
                Creates a tenant-scoped customer and auto-seeds a loyalty account.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form
                className="space-y-3"
                onSubmit={createForm.handleSubmit((values) =>
                  createMutation.mutate(values),
                )}
              >
                <div className="space-y-2">
                  <Label>Full name</Label>
                  <Input
                    {...createForm.register("full_name")}
                    placeholder="Hessa Al-Mansouri"
                  />
                </div>
                <div className="space-y-2">
                  <Label>Email</Label>
                  <Input
                    {...createForm.register("email")}
                    placeholder="hessa@example.com"
                  />
                </div>
                <div className="space-y-2">
                  <Label>Phone</Label>
                  <Input
                    {...createForm.register("phone")}
                    placeholder="+971 50 000 0000"
                  />
                </div>
                <div className="space-y-2">
                  <Label>Notes</Label>
                  <Textarea
                    {...createForm.register("notes")}
                    placeholder="VIP walk-in customer"
                  />
                </div>
                <Button
                  type="submit"
                  className="w-full"
                  loading={createMutation.isPending}
                >
                  Create customer
                </Button>
                {createMutation.data ? (
                  <p className="text-xs text-[var(--muted-foreground)]">
                    Created {createMutation.data.full_name}. Open the profile
                    with ID <code>{createMutation.data.id}</code>.
                  </p>
                ) : null}
              </form>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Open by ID</CardTitle>
              <CardDescription>
                Jump to a customer profile by UUID.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <form className="space-y-3">
                <div className="space-y-2">
                  <Label>Customer ID</Label>
                  <Input
                    {...lookupForm.register("customerId")}
                    placeholder="UUID"
                  />
                </div>
                <Button asChild className="w-full">
                  <Link
                    href={customerId ? `/customers/${customerId}` : "#"}
                  >
                    Open profile
                  </Link>
                </Button>
              </form>
            </CardContent>
          </Card>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>Customers</CardTitle>
            <CardDescription>
              {data?.total ?? 0} customers · server-paginated
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ServerDataTable<CustomerListItem>
              data={data?.items ?? []}
              total={data?.total ?? 0}
              page={page}
              pageSize={pageSize}
              onPageChange={setPage}
              onSearchChange={handleSearchChange}
              searchPlaceholder="Search by email or name..."
              columns={customerColumns}
              isLoading={isLoading}
            />
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
