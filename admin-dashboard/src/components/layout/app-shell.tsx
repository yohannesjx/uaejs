"use client";

import * as Collapsible from "@radix-ui/react-collapsible";
import * as Dialog from "@radix-ui/react-dialog";
import { Command } from "cmdk";
import {
  BarChart3,
  Boxes,
  ChevronDown,
  ChevronRight,
  Command as CommandIcon,
  CreditCard,
  GiftIcon,
  History,
  ImagePlus,
  Layers,
  LogOut,
  Moon,
  RefreshCcw,
  ShoppingBag,
  Sun,
  Truck,
  Users,
  Warehouse,
  Menu,
  X,
} from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { useTheme } from "next-themes";

import { PermissionGate } from "@/components/permission-gate";
import {
  Avatar,
  Badge,
  Button,
  Card,
  Input,
  Separator,
} from "@/components/ui/primitives";
import { cn } from "@/lib/utils";
import { useAuth } from "@/providers/auth-provider";
import { MediaLibraryModal } from "@/components/media/media-library-modal";

type NavItem = {
  title: string;
  href: string;
  action?: string;
  icon: React.ComponentType<{ className?: string }>;
  permission?: string | string[];
  description: string;
};

const navItems: NavItem[] = [
  {
    title: "Dashboard",
    href: "/dashboard",
    icon: BarChart3,
    permission: "analytics.view",
    description: "KPIs, trends, and operational health",
  },
  {
    title: "Products",
    href: "/products",
    icon: ShoppingBag,
    description: "Catalog tooling and pricing setup",
  },
  {
    title: "Orders",
    href: "/orders",
    icon: Boxes,
    description: "Order lookup and invoice tools",
  },
  {
    title: "Warehouses",
    href: "/warehouses",
    icon: Warehouse,
    permission: "inventory.manage",
    description: "Locations, stock, and transfers",
  },
  {
    title: "Customers",
    href: "/customers",
    icon: Users,
    description: "Profiles, points, and loyalty actions",
  },
  {
    title: "Returns",
    href: "/returns",
    description: "RMA approvals and QC workflows",
    icon: RefreshCcw,
  },
  {
    title: "Suppliers",
    href: "/suppliers",
    icon: Truck,
    permission: "suppliers.manage",
    description: "Vendors and procurement",
  },
  {
    title: "Channels",
    href: "/channels",
    icon: CommandIcon,
    permission: "channels.manage",
    description: "Marketplace and sync management",
  },
  {
    title: "Users",
    href: "/users",
    icon: Users,
    permission: "users.manage",
    description: "Admin users, roles, and sessions",
  },
  {
    title: "Media",
    href: "#",
    action: "OPEN_MEDIA",
    icon: ImagePlus,
    permission: "products.read",
    description: "Global media library",
  },
  {
    title: "Activity Log",
    href: "/activity-log",
    icon: History,
    permission: "analytics.view",
    description: "Audit trail of admin actions",
  },
];

const productSubItems = [
  { title: "Products", href: "/products", icon: ShoppingBag },
  { title: "Categories", href: "/products/categories", icon: Boxes },
  { title: "Collections", href: "/collections", icon: Layers },
  { title: "Inventory", href: "/products/inventory", icon: Warehouse },
  { title: "Purchase Orders", href: "/products/purchase-orders", icon: CreditCard },
  { title: "Transfers", href: "/products/transfers", icon: Truck },
  { title: "Gift Cards", href: "/products/gift-cards", icon: GiftIcon },
];

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const router = useRouter();
  const { theme, setTheme } = useTheme();
  const { user, logout, hasPermission } = useAuth();
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [mediaOpen, setMediaOpen] = useState(false);
  const [mobileNavOpen, setMobileNavOpen] = useState(false);
  const [productsOpen, setProductsOpen] = useState(() => {
    if (typeof window === "undefined") return false;
    return localStorage.getItem("nav_products_open") === "true";
  });

  const toggleProducts = (open: boolean) => {
    setProductsOpen(open);
    localStorage.setItem("nav_products_open", String(open));
  };

  const isProductsActive =
    pathname.startsWith("/products") || pathname.startsWith("/collections");

  const isSubNavActive = (href: string) =>
    pathname === href || pathname.startsWith(`${href}/`);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPaletteOpen((current) => !current);
      }
    };

    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, []);

  const paletteItems = useMemo(
    () => navItems.filter((item) => hasPermission(item.permission)),
    [hasPermission],
  );

  // Close mobile nav when pathname changes
  useEffect(() => {
    setMobileNavOpen(false);
  }, [pathname]);

  const renderSidebar = () => (
    <>
      <div className="flex items-center justify-between px-5 py-5">
            <div>
              <p className="text-xs uppercase tracking-[0.24em] text-[var(--muted-foreground)]">
                Dubai Retail OS
              </p>
              <h1 className="mt-2 text-lg font-semibold">Admin Dashboard</h1>
            </div>
            <Badge>RBAC</Badge>
          </div>

          <div className="px-3 pb-3">
            <button
              type="button"
              onClick={() => setPaletteOpen(true)}
              className="flex w-full items-center justify-between rounded-xl border border-[var(--border)] bg-[var(--panel)] px-3 py-2 text-sm text-[var(--muted-foreground)]"
            >
              <span>Search anything...</span>
              <span className="rounded-md border border-[var(--border)] px-1.5 py-0.5 text-xs">
                ⌘K
              </span>
            </button>
          </div>

          <nav className="space-y-1 px-3">
            {navItems.map((item) => {
              // Replace the Products item with a collapsible tree
              if (item.href === "/products") {
                return (
                  <PermissionGate key={item.href} permission={item.permission}>
                    <Collapsible.Root
                      open={productsOpen}
                      onOpenChange={toggleProducts}
                    >
                      <Collapsible.Trigger asChild>
                        <button
                          type="button"
                          className={cn(
                            "flex w-full items-center justify-between rounded-xl px-3 py-2.5 text-sm transition",
                            isProductsActive
                              ? "bg-[#e5e7eb] text-black dark:bg-[#374151] dark:text-white"
                              : "text-[var(--muted-foreground)] hover:bg-[var(--panel)] hover:text-[var(--foreground)]",
                          )}
                        >
                          <span className="flex items-center gap-3">
                            <ShoppingBag className="size-4" />
                            Products
                          </span>
                          <ChevronDown
                            className={cn(
                              "size-4 transition-transform",
                              productsOpen && "rotate-180",
                            )}
                          />
                        </button>
                      </Collapsible.Trigger>

                      <Collapsible.Content className="overflow-hidden data-[state=open]:animate-none">
                        <div className="ml-3 mt-1 space-y-0.5 border-l border-[var(--border)] pl-3">
                          {productSubItems.map((sub) => (
                            <Link
                              key={sub.href}
                              href={sub.href}
                              className={cn(
                                "flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm transition",
                                isSubNavActive(sub.href)
                                  ? "bg-[#e5e7eb] text-black font-medium dark:bg-[#374151] dark:text-white"
                                  : "text-[var(--muted-foreground)] hover:bg-[var(--panel)] hover:text-[var(--foreground)]",
                              )}
                            >
                              <sub.icon className="size-3.5" />
                              {sub.title}
                            </Link>
                          ))}
                        </div>
                      </Collapsible.Content>
                    </Collapsible.Root>
                  </PermissionGate>
                );
              }

              // Skip old Warehouses sub-items that are now under Products
              if (['/warehouses'].includes(item.href)) return null;

              if (item.action === "OPEN_MEDIA") {
                return (
                  <PermissionGate key={item.title} permission={item.permission}>
                    <button
                      type="button"
                      onClick={() => setMediaOpen(true)}
                      className="flex w-full items-center justify-between rounded-xl px-3 py-2.5 text-sm transition text-[var(--muted-foreground)] hover:bg-[var(--panel)] hover:text-[var(--foreground)]"
                    >
                      <span className="flex items-center gap-3">
                        <item.icon className="size-4" />
                        {item.title}
                      </span>
                    </button>
                  </PermissionGate>
                );
              }

              return (
                <PermissionGate key={item.href} permission={item.permission}>
                  <Link
                    href={item.href}
                    className={cn(
                      "flex items-center justify-between rounded-xl px-3 py-2.5 text-sm transition",
                      pathname === item.href
                        ? "bg-[#e5e7eb] text-black dark:bg-[#374151] dark:text-white"
                        : "text-[var(--muted-foreground)] hover:bg-[var(--panel)] hover:text-[var(--foreground)]",
                    )}
                  >
                    <span className="flex items-center gap-3">
                      <item.icon className="size-4" />
                      {item.title}
                    </span>
                    <ChevronRight className="size-4 opacity-60" />
                  </Link>
                </PermissionGate>
              );
            })}
          </nav>

          <div className="mt-auto p-4">
            <Card className="p-4">
              <div className="flex items-center gap-3">
                <Avatar name={user?.full_name} />
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">{user?.full_name ?? "Admin"}</p>
                  <p className="truncate text-xs text-[var(--muted-foreground)]">
                    {user?.email ?? "Not signed in"}
                  </p>
                </div>
              </div>
              <Separator className="my-4" />
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  className="flex-1"
                  onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
                >
                  {theme === "dark" ? (
                    <Sun className="size-4" />
                  ) : (
                    <Moon className="size-4" />
                  )}
                  Theme
                </Button>
                <Button variant="outline" size="sm" className="flex-1" onClick={() => void logout()}>
                  <LogOut className="size-4" />
                  Logout
                </Button>
              </div>
            </Card>
          </div>
    </>
  );

  return (
    <div className="min-h-screen bg-[var(--background)] text-[var(--foreground)]">
      <div className="grid min-h-screen lg:grid-cols-[280px_1fr]">
        <aside className="hidden border-r border-[var(--border)] bg-[var(--sidebar)] lg:flex lg:flex-col">
          {renderSidebar()}
        </aside>

        <main className="min-w-0">
          <div className="sticky top-0 z-20 border-b border-[var(--border)] bg-[var(--background)]/85 backdrop-blur">
            <div className="flex items-center justify-between gap-4 px-4 py-4 md:px-6 lg:px-8">
              <div className="flex items-center gap-3">
                <Button variant="ghost" size="icon" className="lg:hidden -ml-2 text-[var(--muted-foreground)] hover:text-black hover:bg-[var(--muted)]" onClick={() => setMobileNavOpen(true)}>
                  <Menu className="size-5" />
                </Button>
                <div>
                  <p className="hidden md:block text-xs uppercase tracking-[0.22em] text-[var(--muted-foreground)]">
                    Multi-tenant admin
                  </p>
                  <h2 className="text-lg font-semibold">
                    {pathname.startsWith("/collections")
                      ? "Collections"
                      : navItems.find(
                          (item) =>
                            item.href !== "#" && pathname.startsWith(item.href),
                        )?.title ?? "Operations"}
                  </h2>
                </div>
              </div>

              <div className="flex items-center gap-2">
                <Button variant="outline" size="sm" onClick={() => setPaletteOpen(true)}>
                  <CommandIcon className="size-4" />
                  Command
                </Button>
                <Button
                  variant="outline"
                  size="icon"
                  onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
                >
                  {theme === "dark" ? (
                    <Sun className="size-4" />
                  ) : (
                    <Moon className="size-4" />
                  )}
                </Button>
              </div>
            </div>
          </div>

          <div className="px-4 py-6 md:px-6 lg:px-8">{children}</div>
        </main>
      </div>

      <Dialog.Root open={mobileNavOpen} onOpenChange={setMobileNavOpen}>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 z-40 bg-black/45 backdrop-blur-sm lg:hidden" />
          <Dialog.Content className="fixed bottom-0 left-0 top-0 z-50 flex w-[280px] flex-col bg-[var(--sidebar)] shadow-2xl lg:hidden data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:slide-out-to-left data-[state=open]:slide-in-from-left">
            <Dialog.Title className="sr-only">Mobile Navigation</Dialog.Title>
            <Button
              variant="ghost"
              size="icon"
              className="absolute right-4 top-4 text-[var(--muted-foreground)] hover:text-black hover:bg-[var(--muted)]"
              onClick={() => setMobileNavOpen(false)}
            >
              <X className="size-5" />
            </Button>
            {renderSidebar()}
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>

      <Dialog.Root open={paletteOpen} onOpenChange={setPaletteOpen}>
        <Dialog.Portal>
          <Dialog.Overlay className="fixed inset-0 z-40 bg-black/45 backdrop-blur-sm" />
          <Dialog.Content className="fixed left-1/2 top-[18%] z-50 w-[min(680px,calc(100%-2rem))] -translate-x-1/2 overflow-hidden rounded-2xl border border-[var(--border)] bg-[var(--panel)] shadow-2xl">
            <Dialog.Title className="sr-only">Command palette</Dialog.Title>
            <Command className="w-full">
              <div className="border-b border-[var(--border)] p-3">
                <Command.Input
                  asChild
                  autoFocus
                >
                  <Input placeholder="Jump to products, orders, warehouses, analytics..." />
                </Command.Input>
              </div>
              <Command.List className="max-h-[420px] overflow-auto p-2">
                <Command.Empty className="px-3 py-8 text-sm text-[var(--muted-foreground)]">
                  No matching destination.
                </Command.Empty>
                <Command.Group heading="Navigation" className="text-sm">
                  {paletteItems.map((item) => (
                    <Command.Item
                      key={item.href}
                      value={`${item.title} ${item.description}`}
                      onSelect={() => {
                        setPaletteOpen(false);
                        router.push(item.href);
                      }}
                      className="flex cursor-pointer items-center justify-between rounded-xl px-3 py-3 text-sm outline-none data-[selected=true]:bg-[var(--muted)]"
                    >
                      <span className="flex items-center gap-3">
                        <item.icon className="size-4" />
                        <span>
                          <span className="block font-medium">{item.title}</span>
                          <span className="text-xs text-[var(--muted-foreground)]">
                            {item.description}
                          </span>
                        </span>
                      </span>
                      <ChevronRight className="size-4 text-[var(--muted-foreground)]" />
                    </Command.Item>
                  ))}
                </Command.Group>
                <Command.Group heading="Quick actions" className="text-sm">
                  {hasPermission("products.read") && (
                    <Command.Item
                      value="Create product"
                      onSelect={() => {
                        setPaletteOpen(false);
                        router.push("/products");
                      }}
                      className="flex cursor-pointer items-center gap-3 rounded-xl px-3 py-3 text-sm outline-none data-[selected=true]:bg-[var(--muted)]"
                    >
                      <ShoppingBag className="size-4" />
                      <span>Create product</span>
                    </Command.Item>
                  )}
                  {hasPermission("products.read") && (
                    <Command.Item
                      value="Collections curated groups"
                      onSelect={() => {
                        setPaletteOpen(false);
                        router.push("/collections");
                      }}
                      className="flex cursor-pointer items-center gap-3 rounded-xl px-3 py-3 text-sm outline-none data-[selected=true]:bg-[var(--muted)]"
                    >
                      <Layers className="size-4" />
                      <span>Collections</span>
                    </Command.Item>
                  )}
                  {hasPermission("inventory.manage") && (
                    <Command.Item
                      value="Create warehouse"
                      onSelect={() => {
                        setPaletteOpen(false);
                        router.push("/warehouses");
                      }}
                      className="flex cursor-pointer items-center gap-3 rounded-xl px-3 py-3 text-sm outline-none data-[selected=true]:bg-[var(--muted)]"
                    >
                      <Warehouse className="size-4" />
                      <span>Create warehouse</span>
                    </Command.Item>
                  )}
                  {hasPermission("analytics.view") && (
                    <Command.Item
                      value="View activity log"
                      onSelect={() => {
                        setPaletteOpen(false);
                        router.push("/activity-log");
                      }}
                      className="flex cursor-pointer items-center gap-3 rounded-xl px-3 py-3 text-sm outline-none data-[selected=true]:bg-[var(--muted)]"
                    >
                      <History className="size-4" />
                      <span>View activity log</span>
                    </Command.Item>
                  )}
                </Command.Group>
              </Command.List>
            </Command>
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>

      <MediaLibraryModal open={mediaOpen} onOpenChange={setMediaOpen} />
    </div>
  );
}
