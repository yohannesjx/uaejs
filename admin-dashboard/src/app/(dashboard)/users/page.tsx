"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { createColumnHelper } from "@tanstack/react-table";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { PermissionGate } from "@/components/permission-gate";
import { DataTable } from "@/components/data-table";
import { PageHeader } from "@/components/dashboard-blocks";
import { Badge, Button, Card, CardContent, CardDescription, CardHeader, CardTitle, Input, Label } from "@/components/ui/primitives";
import { api } from "@/lib/api-client";
import { formatDate } from "@/lib/utils";
import type { User } from "@/types/api";

const userColumn = createColumnHelper<User>();

const createUserSchema = z.object({
  email: z.string().email(),
  password: z.string().min(8),
  full_name: z.string().min(2),
  role: z.string().min(2),
});

export default function UsersPage() {
  const queryClient = useQueryClient();
  const usersQuery = useQuery({
    queryKey: ["users"],
    queryFn: api.listUsers,
  });

  const createUserForm = useForm<z.infer<typeof createUserSchema>>({
    resolver: zodResolver(createUserSchema),
    defaultValues: {
      email: "",
      password: "",
      full_name: "",
      role: "manager",
    },
  });

  const createUserMutation = useMutation({
    mutationFn: (values: z.infer<typeof createUserSchema>) =>
      api.createUser({
        email: values.email,
        password: values.password,
        full_name: values.full_name,
        roles: [values.role],
      }),
    onSuccess: async () => {
      createUserForm.reset();
      await queryClient.invalidateQueries({ queryKey: ["users"] });
    },
  });

  const revokeAllMutation = useMutation({
    mutationFn: api.revokeAllSessions,
  });

  return (
    <PermissionGate
      permission="users.manage"
      fallback={
        <Card>
          <CardContent className="p-6 text-sm text-[var(--muted-foreground)]">
            You do not have permission to manage users.
          </CardContent>
        </Card>
      }
    >
      <div className="space-y-8">
        <PageHeader
          title="Users, roles & sessions"
          description="Admin user lifecycle, role assignment, and emergency global token revocation."
          actions={
            <Button
              variant="danger"
              onClick={() => revokeAllMutation.mutate()}
              loading={revokeAllMutation.isPending}
            >
              Revoke all sessions
            </Button>
          }
        />

        <div className="grid gap-6 xl:grid-cols-[1.2fr_0.8fr]">
          <Card>
            <CardHeader>
              <CardTitle>Admin users</CardTitle>
              <CardDescription>
                Permission-aware user table backed by <code>/admin/users</code>.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <DataTable
                data={usersQuery.data ?? []}
                columns={[
                  userColumn.display({
                    id: "name",
                    header: "User",
                    cell: ({ row }) => (
                      <div>
                        <p className="font-medium">{row.original.full_name}</p>
                        <p className="text-xs text-[var(--muted-foreground)]">
                          {row.original.email}
                        </p>
                      </div>
                    ),
                  }),
                  userColumn.accessor("is_active", {
                    header: "Status",
                    cell: ({ getValue }) =>
                      getValue() ? <Badge tone="success">Active</Badge> : <Badge tone="danger">Inactive</Badge>,
                  }),
                  userColumn.accessor("permissions_version", {
                    header: "Perm ver",
                  }),
                  userColumn.accessor("updated_at", {
                    header: "Updated",
                    cell: ({ getValue }) => formatDate(getValue()),
                  }),
                ]}
                searchPlaceholder="Filter users..."
              />
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle>Create user</CardTitle>
              <CardDescription>Create a user and seed one initial role.</CardDescription>
            </CardHeader>
            <CardContent>
              <form
                className="space-y-3"
                onSubmit={createUserForm.handleSubmit((values) =>
                  createUserMutation.mutate(values),
                )}
              >
                <div className="space-y-2">
                  <Label>Full name</Label>
                  <Input {...createUserForm.register("full_name")} />
                </div>
                <div className="space-y-2">
                  <Label>Email</Label>
                  <Input {...createUserForm.register("email")} />
                </div>
                <div className="space-y-2">
                  <Label>Password</Label>
                  <Input type="password" {...createUserForm.register("password")} />
                </div>
                <div className="space-y-2">
                  <Label>Role</Label>
                  <Input {...createUserForm.register("role")} placeholder="admin | manager | warehouse" />
                </div>
                <Button type="submit" className="w-full" loading={createUserMutation.isPending}>
                  Create admin user
                </Button>
              </form>
            </CardContent>
          </Card>
        </div>
      </div>
    </PermissionGate>
  );
}
