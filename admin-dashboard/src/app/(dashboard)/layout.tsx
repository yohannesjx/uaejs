import { AppShell } from "@/components/layout/app-shell";
import { RouteGuard } from "@/components/layout/route-guard";

export default function DashboardLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <RouteGuard>
      <AppShell>{children}</AppShell>
    </RouteGuard>
  );
}
