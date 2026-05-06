"use client";

import { useAuth } from "@/providers/auth-provider";

export function PermissionGate({
  permission,
  children,
  fallback = null,
}: {
  permission?: string | string[] | null;
  children: React.ReactNode;
  fallback?: React.ReactNode;
}) {
  const { hasPermission } = useAuth();

  if (!hasPermission(permission)) {
    return <>{fallback}</>;
  }

  return <>{children}</>;
}
