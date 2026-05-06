"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Fragment, useCallback, useMemo, useState } from "react";
import Link from "next/link";
import { Check, ChevronDown, ChevronRight, Copy, Package, Trash2 } from "lucide-react";
import { toast } from "sonner";

import {
  Button,
  Card,
  CardContent,
  Input,
} from "@/components/ui/primitives";
import { MediaLibraryModal } from "@/components/media/media-library-modal";
import { api } from "@/lib/api-client";
import { formatCurrency } from "@/lib/utils";
import type { InventoryListItem, MediaAsset, ProductCategory } from "@/types/api";

type VariantDraft = {
  id: string;
  sku: string;
  color?: string;
  size?: string;
  image_url?: string;
  price?: string;
  sale_price?: string;
  quantity?: string;
};

type ProductMediaTarget = {
  kind: "product";
  productId: string;
  sku?: string;
};

type VariantMediaTarget = {
  kind: "variant";
  variantId: string;
};

export default function ProductsPage() {
  const queryClient = useQueryClient();
  const [page, setPage] = useState(1);
  const [search, setSearch] = useState("");
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});
  const [hoverThumb, setHoverThumb] = useState<string | null>(null);
  const [mediaTarget, setMediaTarget] = useState<ProductMediaTarget | VariantMediaTarget | null>(null);
  const [mediaModalOpen, setMediaModalOpen] = useState(false);
  const [selectedProducts, setSelectedProducts] = useState<Record<string, boolean>>({});
  const [confirmBulkDeleteOpen, setConfirmBulkDeleteOpen] = useState(false);
  const [assignCategoriesOpen, setAssignCategoriesOpen] = useState(false);
  const [selectedCategoryIds, setSelectedCategoryIds] = useState<string[]>([]);
  const [editingRows, setEditingRows] = useState<Record<string, { title: string; price: string; sale_price?: string; stock: string }>>({});
  const [editingPrice, setEditingPrice] = useState<string | null>(null);
  const [editingSalePrice, setEditingSalePrice] = useState<string | null>(null);
  const [variantCellEdit, setVariantCellEdit] = useState<{ variantId: string; field: "sku" | "price" | "sale_price" | "quantity" | "size" | "color" } | null>(null);
  const [hoverInventoryProduct, setHoverInventoryProduct] = useState<string | null>(null);
  const pageSize = 25;

  const { data, isLoading } = useQuery({
    queryKey: ["products", page, pageSize, search],
    queryFn: () =>
      api.listProducts({ page, page_size: pageSize, search: search || undefined }),
  });

  const { data: draftsData } = useQuery({
    queryKey: ["products", "drafts"],
    queryFn: () => api.listProducts({ status: "draft", page_size: 10 }),
  });
  const drafts = draftsData?.items ?? [];
  const products = useMemo(() => data?.items ?? [], [data?.items]);
  const selectedIds = useMemo(
    () => products.filter((p) => selectedProducts[p.product_id]).map((p) => p.product_id),
    [products, selectedProducts],
  );
  const allSelected = products.length > 0 && selectedIds.length === products.length;

  const handleSearchChange = useCallback((s: string) => {
    setSearch(s);
    setPage(1);
  }, []);

  const allExpanded = useMemo(
    () => products.length > 0 && products.every((p) => expanded[p.product_id]),
    [products, expanded],
  );

  const variantQueries = useQuery({
    queryKey: ["products-list-expanded-details", products.map((p) => p.product_id).join(","), expanded],
    queryFn: async () => {
      const ids = products.filter((p) => expanded[p.product_id]).map((p) => p.product_id);
      const out: Record<string, VariantDraft[]> = {};
      await Promise.all(
        ids.map(async (id) => {
          const detail = await api.getProduct(id);
          const rawVariants = (detail.variants ?? []) as Record<string, unknown>[];
          out[id] = rawVariants.map((v) => ({
            id: String(v.id),
            sku: String(v.sku ?? ""),
            color: v.color as string || "",
            size: v.size as string || "",
            image_url: v.image_url as string || "",
            price: v.price && !isNaN(parseFloat(String(v.price))) ? parseFloat(String(v.price)).toFixed(2) : String(v.price ?? ""),
            sale_price: v.sale_price && !isNaN(parseFloat(String(v.sale_price))) ? parseFloat(String(v.sale_price)).toFixed(2) : String(v.sale_price ?? ""),
            quantity: String(v.quantity ?? ""),
          }));
        }),
      );
      return out;
    },
    enabled: products.some((p) => expanded[p.product_id]),
  });
  const { data: categories = [] } = useQuery({
    queryKey: ["product-categories"],
    queryFn: () => api.listCategories(),
    enabled: assignCategoriesOpen,
  });
  const { data: warehouses = [] } = useQuery({
    queryKey: ["warehouses"],
    queryFn: () => api.listWarehouses(),
  });
  const defaultWarehouse = warehouses[0];
  const { data: inventoryRowsResp } = useQuery({
    queryKey: ["inventory-rows-products-page"],
    queryFn: () => api.listInventoryRows(),
  });
  const inventoryRows = inventoryRowsResp?.items ?? [];

  const [variantEdits, setVariantEdits] = useState<Record<string, VariantDraft[]>>({});

  const patchVariant = useMutation({
    mutationFn: async (v: VariantDraft) =>
      api.patchVariant(v.id, {
        sku: v.sku,
        color: v.color || undefined,
        size: v.size || undefined,
        image_url: v.image_url || undefined,
        price: v.price || undefined,
        sale_price: v.sale_price || undefined,
        quantity: v.quantity === undefined || v.quantity === "" ? undefined : Number(v.quantity),
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["products"] });
      queryClient.invalidateQueries({ queryKey: ["products-list-expanded-details"] });
    },
  });

  const removeVariant = useMutation({
    mutationFn: async ({ id }: { id: string; productId: string }) => api.deleteVariant(id),
    onSuccess: (_data, vars) => {
      setVariantEdits((prev) => {
        const list = prev[vars.productId];
        if (!list) return prev;
        return {
          ...prev,
          [vars.productId]: list.filter((v) => v.id !== vars.id),
        };
      });
      queryClient.invalidateQueries({ queryKey: ["products"] });
      queryClient.invalidateQueries({ queryKey: ["products-list-expanded-details"] });
      toast.success("Variant deleted");
    },
    onError: () => {
      toast.error("Failed to delete variant");
    },
  });
  const adjustInventory = useMutation({
    mutationFn: async ({
      warehouseId,
      variantId,
      adjustmentType,
      quantity,
    }: {
      warehouseId: string;
      variantId: string;
      adjustmentType: "increase" | "decrease";
      quantity: number;
    }) =>
      api.adjustInventory({
        warehouse_id: warehouseId,
        variant_id: variantId,
        adjustment_type: adjustmentType,
        quantity,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["inventory-rows-products-page"] });
      queryClient.invalidateQueries({ queryKey: ["products"] });
      queryClient.invalidateQueries({ queryKey: ["products-list-expanded-details"] });
    },
  });

  const toggleStatus = useMutation({
    mutationFn: async ({ id, active }: { id: string; active: boolean }) =>
      api.updateProduct(id, { status: active ? "active" : "draft" }),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["products"] }),
  });

  const duplicateProduct = useMutation({
    mutationFn: async (id: string) => api.duplicateProduct(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["products"] }),
  });
  const deleteProductMutation = useMutation({
    mutationFn: async (id: string) => api.deleteProduct(id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["products"] }),
  });

  const attachProductImage = useMutation({
    mutationFn: async ({
      productId,
      sku,
      imageUrl,
    }: {
      productId: string;
      sku?: string;
      imageUrl: string;
    }) =>
      api.upsertDefaultVariant(productId, {
        sku: sku && sku.trim() ? sku : `SKU-${productId.slice(0, 8).toUpperCase()}`,
        image_url: imageUrl,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["products"] });
      queryClient.invalidateQueries({ queryKey: ["products-list-expanded-details"] });
    },
  });

  const editRows = variantQueries.data ?? {};
  const getVariants = (productId: string) => variantEdits[productId] ?? editRows[productId] ?? [];
  const getVariantById = (productId: string, variantId: string) =>
    getVariants(productId).find((x) => x.id === variantId);
  const rowEditor = (row: { product_id: string; name: string; price: string; stock: number; sale_price?: string }) =>
    editingRows[row.product_id] ?? { title: row.name, price: row.price, stock: String(row.stock), sale_price: row.sale_price ?? "" };
  const inventoryByProduct = useMemo(() => {
    const map = new Map<string, InventoryListItem[]>();
    inventoryRows.forEach((row) => {
      const list = map.get(row.product_id) ?? [];
      list.push(row);
      map.set(row.product_id, list);
    });
    return map;
  }, [inventoryRows]);
  const inventoryByVariantDefaultWarehouse = useMemo(() => {
    const map = new Map<string, number>();
    if (!defaultWarehouse) return map;
    inventoryRows
      .filter((r) => r.warehouse_id === defaultWarehouse.id)
      .forEach((r) => map.set(r.variant_id, r.available_quantity));
    return map;
  }, [inventoryRows, defaultWarehouse]);

  const saveProductPrice = async (row: { product_id: string }, field: "price" | "sale_price", value: string) => {
    // 1. Update main product (if your backend supports sale_price on product, pass it, otherwise just price)
    api.updateProduct(row.product_id, { [field]: value });
    // 2. Cascade down to variants locally and via API
    const vars = getVariants(row.product_id);
    if (vars.length > 0) {
      setVariantEdits((prev) => {
        const next = [...vars];
        const updated = next.map((v) => ({ ...v, [field]: value }));
        return { ...prev, [row.product_id]: updated };
      });
      vars.forEach((v) => {
        patchVariant.mutate({ ...v, [field]: value });
      });
    }
    queryClient.invalidateQueries({ queryKey: ["products"] });
  };
  const applyBulkDelete = async () => {
    if (selectedIds.length === 0) return;
    try {
      await Promise.all(selectedIds.map((id) => deleteProductMutation.mutateAsync(id)));
      setSelectedProducts({});
      setConfirmBulkDeleteOpen(false);
      toast.success(`${selectedIds.length} product(s) deleted`);
      queryClient.invalidateQueries({ queryKey: ["products"] });
    } catch {
      toast.error("Failed to delete selected products");
    }
  };
  const applyBulkCategories = async () => {
    if (selectedIds.length === 0 || selectedCategoryIds.length === 0) return;
    const map = new Map((categories as ProductCategory[]).map((c) => [c.id, c.title]));
    const categoryValue = selectedCategoryIds.map((id) => map.get(id)).filter(Boolean).join(", ");
    try {
      await Promise.all(selectedIds.map((id) => api.updateProduct(id, { category: categoryValue })));
      setAssignCategoriesOpen(false);
      setSelectedCategoryIds([]);
      toast.success(`Assigned categories to ${selectedIds.length} product(s)`);
      queryClient.invalidateQueries({ queryKey: ["products"] });
    } catch {
      toast.error("Failed to assign categories");
    }
  };

  const isEmpty = !isLoading && (data?.total ?? 0) === 0 && !search;

  return (
    <div className="space-y-6">
      {/* Header */}
      {selectedIds.length > 0 && (
        <div className="sticky top-2 z-20 flex items-center justify-between rounded-xl border border-[var(--border)] bg-[var(--panel)] p-3">
          <span className="text-sm font-medium">{selectedIds.length} selected</span>
          <div className="flex items-center gap-2">
            <Button variant="danger" onClick={() => setConfirmBulkDeleteOpen(true)}>
              Delete Selected
            </Button>
            <Button variant="outline" onClick={() => setAssignCategoriesOpen(true)}>
              Assign Categories ({selectedIds.length})
            </Button>
          </div>
        </div>
      )}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Products</h1>
          <p className="text-sm text-[var(--muted-foreground)]">
            Manage your catalog, variants, and pricing.
          </p>
        </div>
        <Button asChild>
          <Link href="/products/new">
            Add product
          </Link>
        </Button>
      </div>

      {/* Unfinished drafts */}
      {drafts.length > 0 && (
        <div className="mb-6 space-y-3">
          <h2 className="text-sm font-medium text-[var(--muted-foreground)]">Unfinished Drafts</h2>
          <div className="flex gap-4 overflow-x-auto pb-2">
            {drafts.map((d) => (
              <Link key={d.id} href={`/products/${d.product_id}/edit`} className="block min-w-[240px] shrink-0 rounded-xl border border-dashed border-[var(--border)] bg-[var(--card)] p-4 transition-colors hover:bg-[var(--muted)]/50">
                <div className="font-medium text-sm text-[var(--foreground)] truncate">{d.name || "Untitled Product"}</div>
                <div className="mt-1 text-xs text-[var(--muted-foreground)]">Recover un-published draft</div>
              </Link>
            ))}
          </div>
        </div>
      )}

      {/* Empty state */}
      {isEmpty ? (
        <div className="flex min-h-[420px] flex-col items-center justify-center rounded-2xl border border-dashed border-[var(--border)] bg-[var(--panel)] px-8 py-16 text-center">
          <div className="mb-6 flex size-16 items-center justify-center rounded-2xl bg-[var(--muted)]">
            <Package className="size-8 text-[var(--muted-foreground)]" />
          </div>
          <h2 className="mb-2 text-lg font-semibold">Add your products</h2>
          <p className="mb-8 max-w-sm text-sm text-[var(--muted-foreground)]">
            Start by stocking your store with products your customers will love.
          </p>
          <Button asChild size="lg">
            <Link href="/products/new">Add your products</Link>
          </Button>
        </div>
      ) : (
        <Card>
          <CardContent className="space-y-4 p-4">
            <div className="flex items-center gap-2">
              <Input
                placeholder="Search by name or SKU…"
                value={search}
                onChange={(e) => handleSearchChange(e.target.value)}
              />
              <Button
                variant="outline"
                onClick={() => {
                  const next = !allExpanded;
                  const map: Record<string, boolean> = {};
                  products.forEach((p) => {
                    map[p.product_id] = next;
                  });
                  setExpanded(map);
                }}
              >
                {allExpanded ? "Collapse all" : "Expand all"}
              </Button>
            </div>

            <div className="overflow-x-auto w-full rounded-xl border border-[var(--border)]">
              <table className="min-w-[1000px] w-full text-sm">
                <thead className="border-b border-[var(--border)] bg-transparent">
                  <tr>
                    <th className="w-10 px-2 py-3 text-center">
                      <input
                        type="checkbox"
                        checked={allSelected}
                        onChange={(e) => {
                          if (e.target.checked) {
                            const next: Record<string, boolean> = {};
                            products.forEach((p) => {
                              next[p.product_id] = true;
                            });
                            setSelectedProducts(next);
                          } else {
                            setSelectedProducts({});
                          }
                        }}
                      />
                    </th>
                    <th className="w-10 px-4 py-3"></th>
                    <th className="px-4 py-3 text-left font-medium text-[var(--muted-foreground)] whitespace-nowrap">Product</th>
                    <th className="px-4 py-3 text-left font-medium text-[var(--muted-foreground)] whitespace-nowrap">SKU</th>
                    <th className="px-4 py-3 text-left font-medium text-[var(--muted-foreground)] whitespace-nowrap">Inventory</th>
                    <th className="px-4 py-3 text-left font-medium text-[var(--muted-foreground)] whitespace-nowrap">Price (AED)</th>
                    <th className="px-4 py-3 text-left font-medium text-[var(--muted-foreground)] whitespace-nowrap">Sale Price (AED)</th>
                    <th className="px-4 py-3 text-center font-medium text-[var(--muted-foreground)] whitespace-nowrap">Status</th>
                    <th className="px-4 py-3 text-right font-medium text-[var(--muted-foreground)] whitespace-nowrap">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-[var(--border)] bg-[var(--panel)]">
                  {products.map((row) => {
                    const isOpen = !!expanded[row.product_id];
                    const variants = getVariants(row.product_id);
                    return (
                      <Fragment key={row.id}>
                        {/* Main Product Row */}
                        <tr className="group transition-colors hover:bg-[var(--muted)]/20">
                          <td className="px-2 py-4 text-center">
                            <input
                              type="checkbox"
                              checked={!!selectedProducts[row.product_id]}
                              onChange={(e) =>
                                setSelectedProducts((prev) => ({ ...prev, [row.product_id]: e.target.checked }))
                              }
                            />
                          </td>
                          <td className="px-2 py-4">
                            <button
                              type="button"
                              className="rounded p-1 text-[var(--muted-foreground)] hover:bg-[var(--muted)] hover:text-black"
                              onClick={() =>
                                setExpanded((prev) => ({ ...prev, [row.product_id]: !prev[row.product_id] }))
                              }
                            >
                              {isOpen ? <ChevronDown className="size-4" /> : <ChevronRight className="size-4" />}
                            </button>
                          </td>
                          <td className="px-4 py-4 whitespace-nowrap">
                            <div className="flex items-center gap-3">
                              <button
                                type="button"
                                className="group/img relative"
                                onClick={() => {
                                  setMediaTarget({ kind: "product", productId: row.product_id, sku: row.sku });
                                  setMediaModalOpen(true);
                                }}
                                title={row.thumbnail ? "Change product image" : "Upload product image"}
                              >
                                {row.thumbnail ? (
                                  <>
                                    <img
                                      src={row.thumbnail}
                                      alt={row.name}
                                      className="size-10 rounded-md border border-[var(--border)] object-cover bg-[var(--muted)]"
                                      onMouseEnter={() => setHoverThumb(row.thumbnail || null)}
                                      onMouseLeave={() => setHoverThumb(null)}
                                    />
                                  </>
                                ) : (
                                  <div className="flex size-10 shrink-0 items-center justify-center rounded-md border border-dashed border-[var(--border)] bg-[var(--muted)] text-[var(--muted-foreground)]">
                                    <Package className="size-4" />
                                  </div>
                                )}
                                <span className="absolute -bottom-5 left-1/2 -translate-x-1/2 text-[10px] text-[var(--muted-foreground)] opacity-0 transition-opacity group-hover/img:opacity-100">
                                  Upload
                                </span>
                              </button>
                              <Link href={`/products/${row.product_id}/edit`} className="font-semibold text-[var(--foreground)] hover:underline">
                                {row.name}
                              </Link>
                            </div>
                          </td>
                          <td className="px-4 py-4">
                            <span className="font-mono text-xs text-[var(--muted-foreground)]">{row.sku || "—"}</span>
                          </td>
                          <td className="px-4 py-4">
                            {(() => {
                              const rows = inventoryByProduct.get(row.product_id) ?? [];
                              const totalAvailable = rows.reduce((acc, curr) => acc + curr.available_quantity, 0);
                              const locationCount = new Set(rows.map((x) => x.warehouse_id)).size;
                              return (
                                <div
                                  className="relative"
                                  onMouseEnter={() => setHoverInventoryProduct(row.product_id)}
                                  onMouseLeave={() => setHoverInventoryProduct(null)}
                                >
                                  <div className="font-medium text-[var(--foreground)]">Available: {totalAvailable} units</div>
                                  {locationCount > 1 && (
                                    <div className="text-xs text-[var(--muted-foreground)]">Across {locationCount} locations</div>
                                  )}
                                  {hoverInventoryProduct === row.product_id && rows.length > 0 && (
                                    <div className="absolute left-0 top-10 z-20 min-w-[220px] rounded-lg border border-[var(--border)] bg-[var(--panel)] p-2 shadow-lg">
                                      {Object.entries(rows.reduce<Record<string, number>>((acc, r) => {
                                        acc[r.warehouse] = (acc[r.warehouse] ?? 0) + r.available_quantity;
                                        return acc;
                                      }, {})).map(([warehouse, qty]) => (
                                        <div key={warehouse} className="flex items-center justify-between gap-3 text-xs">
                                          <span>{warehouse}</span>
                                          <span className="font-medium">{qty}</span>
                                        </div>
                                      ))}
                                    </div>
                                  )}
                                </div>
                              );
                            })()}
                          </td>
                          <td className="px-4 py-4">
                            {editingPrice === row.product_id ? (
                              <div className="flex items-center gap-2">
                                <Input
                                  className="h-8 w-24"
                                  value={rowEditor(row).price}
                                  onChange={(e) =>
                                    setEditingRows((prev) => ({
                                      ...prev,
                                      [row.product_id]: {
                                        ...rowEditor(row),
                                        price: e.target.value,
                                      },
                                    }))
                                  }
                                  onKeyDown={(e) => {
                                    if (e.key === "Enter") {
                                      saveProductPrice(row, "price", rowEditor(row).price);
                                      setEditingPrice(null);
                                    }
                                  }}
                                  autoFocus
                                />
                                <button type="button" className="rounded p-1 text-emerald-600 hover:bg-emerald-50" onClick={() => {
                                  saveProductPrice(row, "price", rowEditor(row).price);
                                  setEditingPrice(null);
                                }}>
                                  <Check className="size-4" />
                                </button>
                              </div>
                            ) : (
                              <button type="button" className="font-medium hover:underline text-[var(--foreground)]" onClick={() => setEditingPrice(row.product_id)}>
                                {formatCurrency(rowEditor(row).price)}
                              </button>
                            )}
                          </td>
                          <td className="px-4 py-4 whitespace-nowrap">
                            {editingSalePrice === row.product_id ? (
                              <div className="flex items-center gap-2">
                                <Input
                                  className="h-8 w-24"
                                  value={rowEditor(row).sale_price || ""}
                                  onChange={(e) =>
                                    setEditingRows((prev) => ({
                                      ...prev,
                                      [row.product_id]: {
                                        ...rowEditor(row),
                                        sale_price: e.target.value,
                                      },
                                    }))
                                  }
                                  onKeyDown={(e) => {
                                    if (e.key === "Enter") {
                                      saveProductPrice(row, "sale_price", rowEditor(row).sale_price || "");
                                      setEditingSalePrice(null);
                                    }
                                  }}
                                  autoFocus
                                />
                                <button type="button" className="rounded p-1 text-emerald-600 hover:bg-emerald-50" onClick={() => {
                                  saveProductPrice(row, "sale_price", rowEditor(row).sale_price || "");
                                  setEditingSalePrice(null);
                                }}>
                                  <Check className="size-4" />
                                </button>
                              </div>
                            ) : (
                              <button type="button" className="text-[var(--muted-foreground)] hover:underline hover:text-black font-medium" onClick={() => setEditingSalePrice(row.product_id)}>
                                {rowEditor(row).sale_price ? formatCurrency(rowEditor(row).sale_price!) : "Add Sale"}
                              </button>
                            )}
                          </td>
                          <td className="px-4 py-4 text-center">
                            <button
                              type="button"
                              aria-label={`Toggle ${row.name} status`}
                              className={`relative inline-flex h-6 w-11 items-center rounded-full transition-colors ${row.status === "active" ? "bg-black" : "bg-slate-200"}`}
                              onClick={() =>
                                toggleStatus.mutate({ id: row.product_id, active: row.status !== "active" })
                              }
                            >
                              <span
                                className={`inline-block size-5 transform rounded-full bg-white transition-transform ${row.status === "active" ? "translate-x-5" : "translate-x-1"}`}
                              />
                            </button>
                          </td>
                          <td className="px-4 py-4 text-right whitespace-nowrap">
                            <button className="rounded p-1 text-[var(--muted-foreground)] hover:bg-[var(--muted)] hover:text-black" onClick={() => duplicateProduct.mutate(row.product_id)}>
                              <Copy className="size-4" />
                            </button>
                          </td>
                        </tr>

                        {/* Variant Sub-rows */}
                        {isOpen && (
                          variants.length > 0 ? variants.map((v) => (
                            <tr key={v.id} className="bg-[var(--muted)]/20 transition-colors hover:bg-[var(--muted)]/35 border-t border-[var(--border)]/50">
                              <td></td>
                              <td></td>
                              <td className="px-4 py-4 whitespace-nowrap">
                                <div className="flex items-center gap-3">
                                  {v.image_url ? (
                                    <div className="group/vimg relative flex items-center justify-center">
                                      <button
                                        type="button"
                                        onClick={() => {
                                          setMediaTarget({ kind: "variant", variantId: v.id });
                                          setMediaModalOpen(true);
                                        }}
                                      >
                                        <img
                                          src={v.image_url}
                                          alt={v.sku}
                                          className="size-8 rounded-md border border-[var(--border)] object-cover bg-[var(--muted)]"
                                          onMouseEnter={() => setHoverThumb(v.image_url || null)}
                                          onMouseLeave={() => setHoverThumb(null)}
                                        />
                                      </button>
                                    </div>
                                  ) : (
                                    <button
                                      type="button"
                                      className="flex size-8 shrink-0 items-center justify-center rounded-md border border-dashed border-[var(--border)] bg-[var(--muted)] text-[var(--muted-foreground)] hover:text-black hover:border-[var(--muted-foreground)]"
                                      onClick={() => {
                                        setMediaTarget({ kind: "variant", variantId: v.id });
                                        setMediaModalOpen(true);
                                      }}
                                      title="Upload variant image"
                                    >
                                      <Package className="size-3" />
                                    </button>
                                  )}
                                  {variantCellEdit?.variantId === v.id && variantCellEdit.field === "sku" ? (
                                    <Input
                                      className="h-8 w-48 font-mono text-xs"
                                      value={v.sku || ""}
                                      onChange={(e) =>
                                        setVariantEdits((prev) => {
                                          const next = [...getVariants(row.product_id)];
                                          const idx = next.findIndex((x) => x.id === v.id);
                                          if (idx >= 0) next[idx] = { ...next[idx], sku: e.target.value };
                                          return { ...prev, [row.product_id]: next };
                                        })
                                      }
                                      onKeyDown={(e) => {
                                        if (e.key === "Enter") {
                                          patchVariant.mutate(getVariantById(row.product_id, v.id) || v);
                                          setVariantCellEdit(null);
                                        }
                                      }}
                                      autoFocus
                                    />
                                  ) : (
                                    <button
                                      type="button"
                                      className="font-mono text-xs text-[var(--muted-foreground)] hover:underline"
                                      onClick={() => setVariantCellEdit({ variantId: v.id, field: "sku" })}
                                    >
                                      {v.sku || "—"}
                                    </button>
                                  )}
                                  {variantCellEdit?.variantId === v.id && variantCellEdit.field === "size" ? (
                                    <Input
                                      className="h-8 w-24"
                                      value={v.size || ""}
                                      onChange={(e) =>
                                        setVariantEdits((prev) => {
                                          const next = [...getVariants(row.product_id)];
                                          const idx = next.findIndex((x) => x.id === v.id);
                                          if (idx >= 0) next[idx] = { ...next[idx], size: e.target.value };
                                          return { ...prev, [row.product_id]: next };
                                        })
                                      }
                                      onKeyDown={(e) => {
                                        if (e.key === "Enter") {
                                          patchVariant.mutate(getVariantById(row.product_id, v.id) || v);
                                          setVariantCellEdit(null);
                                        }
                                      }}
                                      autoFocus
                                    />
                                  ) : (
                                    <button
                                      type="button"
                                      className="inline-flex items-center rounded-md border border-[var(--border)] bg-[var(--muted)]/20 px-2 py-0.5 text-xs font-bold hover:bg-[var(--muted)]"
                                      onClick={() => setVariantCellEdit({ variantId: v.id, field: "size" })}
                                    >
                                      {v.size || "Add Size"}
                                    </button>
                                  )}
                                  {variantCellEdit?.variantId === v.id && variantCellEdit.field === "color" ? (
                                    <Input
                                      className="h-8 w-28"
                                      value={v.color || ""}
                                      onChange={(e) =>
                                        setVariantEdits((prev) => {
                                          const next = [...getVariants(row.product_id)];
                                          const idx = next.findIndex((x) => x.id === v.id);
                                          if (idx >= 0) next[idx] = { ...next[idx], color: e.target.value };
                                          return { ...prev, [row.product_id]: next };
                                        })
                                      }
                                      onKeyDown={(e) => {
                                        if (e.key === "Enter") {
                                          patchVariant.mutate(getVariantById(row.product_id, v.id) || v);
                                          setVariantCellEdit(null);
                                        }
                                      }}
                                      autoFocus
                                    />
                                  ) : (
                                    <button
                                      className="inline-flex items-center rounded-full border border-[var(--border)] bg-[var(--panel)] px-3 py-1 text-xs font-semibold hover:bg-[var(--muted)]"
                                      onClick={() => setVariantCellEdit({ variantId: v.id, field: "color" })}
                                    >
                                      {v.color || "Add Color"}
                                    </button>
                                  )}
                                  <button
                                    className="inline-flex items-center rounded-full border border-[var(--border)] bg-[var(--panel)] px-3 py-1 text-xs font-semibold hover:bg-[var(--muted)]"
                                    onClick={() => {
                                      setMediaTarget({ kind: "variant", variantId: v.id });
                                      setMediaModalOpen(true);
                                    }}
                                  >
                                    Image
                                  </button>
                                </div>
                              </td>
                              <td className="px-4 py-4">
                                <span className="font-mono text-xs text-[var(--muted-foreground)]">{v.sku || "—"}</span>
                              </td>
                              <td className="px-4 py-4">
                                <div className="text-xs text-[var(--muted-foreground)]">
                                  Available: {inventoryByVariantDefaultWarehouse.get(v.id) ?? Number(v.quantity || 0)} units
                                </div>
                              </td>
                              <td className="px-4 py-4">
                                {variantCellEdit?.variantId === v.id && variantCellEdit.field === "price" ? (
                                  <Input
                                    className="h-8 w-24"
                                    value={v.price || row.price}
                                    onChange={(e) =>
                                      setVariantEdits((prev) => {
                                        const next = [...getVariants(row.product_id)];
                                        const idx = next.findIndex((x) => x.id === v.id);
                                        if (idx >= 0) next[idx] = { ...next[idx], price: e.target.value };
                                        return { ...prev, [row.product_id]: next };
                                      })
                                    }
                                    onKeyDown={(e) => {
                                      if (e.key === "Enter") {
                                        patchVariant.mutate(getVariantById(row.product_id, v.id) || v);
                                        setVariantCellEdit(null);
                                      }
                                    }}
                                    autoFocus
                                  />
                                ) : (
                                  <button
                                    type="button"
                                    className="font-medium text-[var(--foreground)] hover:underline"
                                    onClick={() => setVariantCellEdit({ variantId: v.id, field: "price" })}
                                  >
                                    {formatCurrency(v.price || row.price)}
                                  </button>
                                )}
                              </td>
                              <td className="px-4 py-4">
                                {variantCellEdit?.variantId === v.id && variantCellEdit.field === "sale_price" ? (
                                  <Input
                                    className="h-8 w-24"
                                    value={v.sale_price || ""}
                                    onChange={(e) =>
                                      setVariantEdits((prev) => {
                                        const next = [...getVariants(row.product_id)];
                                        const idx = next.findIndex((x) => x.id === v.id);
                                        if (idx >= 0) next[idx] = { ...next[idx], sale_price: e.target.value };
                                        return { ...prev, [row.product_id]: next };
                                      })
                                    }
                                    onKeyDown={(e) => {
                                      if (e.key === "Enter") {
                                        patchVariant.mutate(getVariantById(row.product_id, v.id) || v);
                                        setVariantCellEdit(null);
                                      }
                                    }}
                                    autoFocus
                                  />
                                ) : (
                                  <button
                                    className="text-xs font-medium text-[var(--muted-foreground)] hover:text-black hover:underline"
                                    onClick={() => setVariantCellEdit({ variantId: v.id, field: "sale_price" })}
                                  >
                                    {v.sale_price ? formatCurrency(v.sale_price) : "Add Sale"}
                                  </button>
                                )}
                              </td>
                              <td className="px-4 py-4 text-center">
                                {variantCellEdit?.variantId === v.id && variantCellEdit.field === "quantity" ? (
                                  <div className="flex flex-col items-center justify-center gap-1">
                                    <Input
                                      className="h-8 w-20 text-center"
                                      type="number"
                                      value={v.quantity ?? ""}
                                      onChange={(e) =>
                                        setVariantEdits((prev) => {
                                          const next = [...getVariants(row.product_id)];
                                          const idx = next.findIndex((x) => x.id === v.id);
                                          if (idx >= 0) next[idx] = { ...next[idx], quantity: e.target.value };
                                          return { ...prev, [row.product_id]: next };
                                        })
                                      }
                                      onKeyDown={(e) => {
                                        if (e.key === "Enter") {
                                          const latest = getVariantById(row.product_id, v.id) || v;
                                          const q = parseInt(latest.quantity || "0", 10);
                                          const target = isNaN(q) ? 0 : q;
                                          const current = inventoryByVariantDefaultWarehouse.get(v.id) ?? Number(v.quantity || 0);
                                          const delta = target - current;
                                          if (delta !== 0 && defaultWarehouse) {
                                            adjustInventory.mutate({
                                              warehouseId: defaultWarehouse.id,
                                              variantId: v.id,
                                              adjustmentType: delta > 0 ? "increase" : "decrease",
                                              quantity: Math.abs(delta),
                                            });
                                          }
                                          setVariantCellEdit(null);
                                        }
                                      }}
                                      autoFocus
                                    />
                                    <span className="text-[10px] text-[var(--muted-foreground)]">Updating stock in Default Warehouse</span>
                                  </div>
                                ) : (
                                  <button
                                    type="button"
                                    onClick={() => setVariantCellEdit({ variantId: v.id, field: "quantity" })}
                                    className={`inline-flex items-center rounded-full px-3 py-1 text-xs font-bold shadow-sm transition-colors hover:ring-2 hover:ring-[var(--ring)] hover:ring-offset-1 ${v.quantity && parseInt(v.quantity) > 0 ? "bg-black text-white" : "bg-slate-200 text-slate-800"}`}
                                  >
                                    {(inventoryByVariantDefaultWarehouse.get(v.id) ?? Number(v.quantity || 0)) > 0
                                      ? `${inventoryByVariantDefaultWarehouse.get(v.id) ?? Number(v.quantity || 0)} available`
                                      : "Out of stock"}
                                  </button>
                                )}
                              </td>
                              <td className="px-4 py-4 text-right">
                                <button
                                  className="rounded p-1 text-[var(--muted-foreground)] hover:bg-red-50 hover:text-red-600"
                                  onClick={() => {
                                    if (!window.confirm("Delete this variant?")) return;
                                    removeVariant.mutate({ id: v.id, productId: row.product_id });
                                  }}
                                >
                                  <Trash2 className="size-4" />
                                </button>
                              </td>
                            </tr>
                          )) : (
                            <tr className="bg-[var(--muted)]/20 border-t border-[var(--border)]/50">
                              <td colSpan={9} className="px-4 py-4 text-sm text-[var(--muted-foreground)]">
                                No variants for this product yet.
                              </td>
                            </tr>
                          )
                        )}
                      </Fragment>
                    );
                  })}
                </tbody>
              </table>
            </div>

            <div className="flex items-center justify-between">
              <span className="text-xs text-[var(--muted-foreground)]">Total: {data?.total ?? 0}</span>
              <div className="flex items-center gap-2">
                <Button variant="outline" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
                  Prev
                </Button>
                <span className="text-xs">Page {page}</span>
                <Button variant="outline" size="sm" disabled={(data?.items?.length ?? 0) < pageSize} onClick={() => setPage((p) => p + 1)}>
                  Next
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      <MediaLibraryModal
        open={mediaModalOpen}
        onOpenChange={setMediaModalOpen}
        mode="single"
        value={[]}
        onChange={(assets: MediaAsset[]) => {
          if (!mediaTarget || assets.length === 0) return;
          const chosen = assets[0];
          if (mediaTarget.kind === "product") {
            attachProductImage.mutate({
              productId: mediaTarget.productId,
              sku: mediaTarget.sku,
              imageUrl: chosen.url,
            });
            return;
          }
          const productId = Object.keys(editRows).find((pid) =>
            getVariants(pid).some((v) => v.id === mediaTarget.variantId),
          );
          if (!productId) return;
          const variants = getVariants(productId);
          const idx = variants.findIndex((v) => v.id === mediaTarget.variantId);
          if (idx < 0) return;
          const next = [...variants];
          next[idx] = { ...next[idx], image_url: chosen.url };
          setVariantEdits((prev) => ({ ...prev, [productId]: next }));
          patchVariant.mutate(next[idx]);
        }}
      />

      {confirmBulkDeleteOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <Card className="w-full max-w-md">
            <CardContent className="space-y-4 p-6">
              <h3 className="text-lg font-semibold">Delete selected products?</h3>
              <p className="text-sm text-[var(--muted-foreground)]">
                This will remove {selectedIds.length} selected product(s).
              </p>
              <div className="flex justify-end gap-2">
                <Button variant="outline" onClick={() => setConfirmBulkDeleteOpen(false)}>
                  Cancel
                </Button>
                <Button variant="danger" onClick={applyBulkDelete}>
                  Delete Selected
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      )}

      {assignCategoriesOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4">
          <Card className="w-full max-w-lg">
            <CardContent className="space-y-4 p-6">
              <h3 className="text-lg font-semibold">Assign Categories ({selectedIds.length})</h3>
              <div className="max-h-72 space-y-2 overflow-y-auto rounded border border-[var(--border)] p-3">
                {(categories as ProductCategory[]).map((cat) => (
                  <label key={cat.id} className="flex items-center gap-2 text-sm">
                    <input
                      type="checkbox"
                      checked={selectedCategoryIds.includes(cat.id)}
                      onChange={(e) =>
                        setSelectedCategoryIds((prev) =>
                          e.target.checked ? [...prev, cat.id] : prev.filter((id) => id !== cat.id),
                        )
                      }
                    />
                    <span>{cat.title}</span>
                  </label>
                ))}
              </div>
              <div className="flex justify-end gap-2">
                <Button variant="outline" onClick={() => setAssignCategoriesOpen(false)}>
                  Cancel
                </Button>
                <Button onClick={applyBulkCategories} disabled={selectedCategoryIds.length === 0}>
                  Assign Categories
                </Button>
              </div>
            </CardContent>
          </Card>
        </div>
      )}

      {hoverThumb && (
        <div className="pointer-events-none fixed bottom-4 right-4 z-[70] rounded-xl border border-[var(--border)] bg-[var(--panel)] p-2 shadow-2xl">
          <img src={hoverThumb} alt="Preview" className="h-44 w-44 rounded-md object-cover" />
        </div>
      )}
    </div>
  );
}
