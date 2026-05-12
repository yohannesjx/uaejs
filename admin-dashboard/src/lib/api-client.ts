import {
  clearStoredUser,
  clearTenantId,
  clearTokens,
  getAccessToken,
  getRefreshToken,
  getTenantId,
  setTokens,
} from "@/lib/auth-storage";
import type {
  ActivityLogItem,
  ApiErrorPayload,
  AuthTokenPair,
  ChannelPrice,
  Customer,
  CustomerListItem,
  CustomerProfileResponse,
  ExternalPlatform,
  ForecastResult,
  FraudSignal,
  LoyaltyTransaction,
  Order,
  OrderInvoice,
  OrderListItem,
  PointsRedemptionResult,
  Product,
  ProductCategory,
  ProductCollection,
  ProductCreatePayload,
  ProductDetail,
  ProductListItem,
  Variant,
  PromotionAnalyticsResponse,
  ReorderSuggestion,
  ReturnListItem,
  ReturnRequest,
  Shipment,
  ShipmentEvent,
  ShipmentListItem,
  Supplier,
  Tenant,
  TenantSettings,
  TransferResult,
  User,
  Warehouse,
  WarehouseStock,
  MediaAsset,
  InventoryListItem,
  InventoryTransfer,
} from "@/types/api";

/**
 * API origin for browser calls.
 * When the dashboard is opened via a public host (not localhost), using
 * NEXT_PUBLIC_API_BASE_URL=http://localhost:8080 makes the *visitor's* browser
 * call their own loopback — blocked by Private Network Access / CORS.
 * So if env is missing or still points to loopback while the page is on a
 * real host, we use the same hostname with port 8080.
 */
function getApiBaseUrl(): string {
  const fromEnv = process.env.NEXT_PUBLIC_API_BASE_URL;

  if (typeof window !== "undefined") {
    const h = window.location.hostname;
    const onLoopback = h === "localhost" || h === "127.0.0.1";
    if (!onLoopback) {
      const envLooksLoopback =
        !fromEnv ||
        fromEnv.includes("localhost") ||
        fromEnv.includes("127.0.0.1");
      if (envLooksLoopback) {
        const port = process.env.NEXT_PUBLIC_API_PORT ?? "8080";
        return `${window.location.protocol}//${h}:${port}`;
      }
      return fromEnv;
    }
    if (fromEnv) return fromEnv;
    return `http://localhost:${process.env.NEXT_PUBLIC_API_PORT ?? "8080"}`;
  }

	if (fromEnv) return fromEnv;
	return `http://localhost:${process.env.NEXT_PUBLIC_API_PORT ?? "8080"}`;
}

/**
 * Media/upload URLs stored in the DB often use http://localhost:8080/uploads/...
 * (server default). Browsers refuse to load loopback from a public dashboard
 * (Private Network Access). Rewrites to the same API origin as API calls.
 */
export function publicUploadUrl(stored: string | null | undefined): string {
	if (stored == null || stored === "") return "";
	const raw = String(stored).trim();
	const idx = raw.indexOf("/uploads/");
	if (idx === -1) return raw;
	return `${getApiBaseUrl()}${raw.slice(idx)}`;
}

export class ApiError extends Error {
  status: number;
  payload?: ApiErrorPayload | string;

  constructor(status: number, message: string, payload?: ApiErrorPayload | string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.payload = payload;
  }
}

type RequestInitWithAuth = RequestInit & {
  auth?: boolean;
  retryOnUnauthorized?: boolean;
  tenantId?: string | null;
};

async function parseResponse<T>(response: Response): Promise<T> {
  const contentType = response.headers.get("content-type") ?? "";

  if (!response.ok) {
    if (contentType.includes("application/json")) {
      const payload = (await response.json()) as ApiErrorPayload;
      throw new ApiError(
        response.status,
        payload.error ?? payload.message ?? "Request failed",
        payload,
      );
    }

    const text = await response.text();
    throw new ApiError(response.status, text || "Request failed", text);
  }

  if (contentType.includes("application/json")) {
    return (await response.json()) as T;
  }

  return (await response.text()) as T;
}

async function refreshTokens() {
  const refreshToken = getRefreshToken();
  if (!refreshToken) throw new ApiError(401, "Session expired");

  const response = await fetch(`${getApiBaseUrl()}/auth/refresh`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ refresh_token: refreshToken }),
  });

  const data = await parseResponse<AuthTokenPair>(response);
  setTokens(data.access_token, data.refresh_token);
  return data;
}

export async function apiFetch<T>(
  path: string,
  init: RequestInitWithAuth = {},
): Promise<T> {
  const headers = new Headers(init.headers);
  const accessToken = getAccessToken();
  const tenantId = init.tenantId ?? getTenantId();

  if (!headers.has("Content-Type") && init.body && !(init.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }

  if (init.auth !== false && accessToken) {
    headers.set("Authorization", `Bearer ${accessToken}`);
  }

  if (tenantId) {
    headers.set("X-Tenant-ID", tenantId);
  }

  try {
    const response = await fetch(`${getApiBaseUrl()}${path}`, {
      ...init,
      headers,
    });

    return await parseResponse<T>(response);
  } catch (error) {
    if (
      init.auth !== false &&
      init.retryOnUnauthorized !== false &&
      error instanceof ApiError &&
      error.status === 401 &&
      getRefreshToken()
    ) {
      await refreshTokens();
      return apiFetch<T>(path, { ...init, retryOnUnauthorized: false });
    }

    if (error instanceof ApiError && error.status === 401) {
      clearTokens();
      clearStoredUser();
      clearTenantId();
    }

    throw error;
  }
}

export const api = {
  login: (email: string, password: string) =>
    apiFetch<AuthTokenPair>("/auth/login", {
      method: "POST",
      auth: false,
      body: JSON.stringify({ email, password }),
    }),

  logout: (refreshToken?: string | null) =>
    apiFetch<void>("/auth/logout", {
      method: "POST",
      auth: false,
      body: JSON.stringify({ refresh_token: refreshToken }),
    }),

  me: () => apiFetch<User>("/auth/me"),

  listUsers: () => apiFetch<User[]>("/admin/users"),
  createUser: (payload: {
    email: string;
    password: string;
    full_name: string;
    roles: string[];
  }) =>
    apiFetch<User>("/admin/users", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  updateUser: (id: string, payload: { full_name?: string; is_active?: boolean }) =>
    apiFetch<User>(`/admin/users/${id}`, {
      method: "PATCH",
      body: JSON.stringify(payload),
    }),
  assignRole: (id: string, role: string) =>
    apiFetch<void>(`/admin/users/${id}/roles`, {
      method: "POST",
      body: JSON.stringify({ role }),
    }),
  revokeAllSessions: () =>
    apiFetch<{ message: string; global_auth_version: number }>(
      "/admin/auth/revoke-all",
      { method: "POST" },
    ),

  listTenants: () => apiFetch<Tenant[]>("/admin/tenants"),
  getTenantSettings: (id: string) =>
    apiFetch<TenantSettings>(`/admin/tenants/${id}/settings`),
  saveTenantSettings: (id: string, settings: Record<string, unknown>) =>
    apiFetch<void>(`/admin/tenants/${id}/settings`, {
      method: "PUT",
      body: JSON.stringify(settings),
    }),

  listWarehouses: () => apiFetch<Warehouse[]>("/admin/warehouses"),
  createWarehouse: (payload: Partial<Warehouse>) =>
    apiFetch<Warehouse>("/admin/warehouses", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  updateWarehouse: (id: string, payload: Partial<Warehouse>) =>
    apiFetch<void>(`/admin/warehouses/${id}`, {
      method: "PUT",
      body: JSON.stringify(payload),
    }),
  getWarehouseInventory: (id: string) =>
    apiFetch<WarehouseStock[]>(`/admin/warehouses/${id}/inventory`),
  setWarehouseStock: (
    id: string,
    payload: {
      variant_id: string;
      qty_on_hand: number;
      reorder_point?: number;
      reorder_qty?: number;
    },
  ) =>
    apiFetch<void>(`/admin/warehouses/${id}/stock`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  transferWarehouseStock: (payload: {
    from_warehouse_id: string;
    to_warehouse_id: string;
    variant_id: string;
    quantity: number;
    notes?: string;
  }) =>
    apiFetch<TransferResult>("/admin/warehouses/transfer", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  listCustomers: (params?: {
    page?: number;
    page_size?: number;
    search?: string;
    tier?: string;
    email?: string;
  }) => {
    const sp = new URLSearchParams();
    if (params?.page) sp.set("page", String(params.page));
    if (params?.page_size) sp.set("page_size", String(params.page_size));
    if (params?.search) sp.set("search", params.search);
    if (params?.tier) sp.set("tier", params.tier);
    if (params?.email) sp.set("email", params.email);
    return apiFetch<{ items: CustomerListItem[]; total: number }>(
      `/customers?${sp.toString()}`,
    );
  },
  createCustomer: (payload: Partial<Customer>) =>
    apiFetch<Customer>("/customers/", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  getCustomer: (id: string) =>
    apiFetch<CustomerProfileResponse>(`/customers/${id}`),
  addCustomerPoints: (id: string, payload: { order_id: string; total_aed: string }) =>
    apiFetch<LoyaltyTransaction>(`/customers/${id}/points/add`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  redeemCustomerPoints: (id: string, points_to_redeem: number) =>
    apiFetch<PointsRedemptionResult>(`/customers/${id}/points/redeem`, {
      method: "POST",
      body: JSON.stringify({ points_to_redeem }),
    }),
  getCustomerPointsHistory: (id: string, limit = 50) =>
    apiFetch<LoyaltyTransaction[]>(`/customers/${id}/points/history?limit=${limit}`),

  listSuppliers: () => apiFetch<Supplier[]>("/admin/suppliers"),
  listChannels: () => apiFetch<ExternalPlatform[]>("/admin/channels"),

  getForecast: (sku: string, channel?: string) =>
    apiFetch<ForecastResult>(
      `/admin/analytics/forecast?sku=${encodeURIComponent(sku)}${channel ? `&channel=${encodeURIComponent(channel)}` : ""
      }`,
    ),
  getReorders: () => apiFetch<ReorderSuggestion[]>("/admin/analytics/reorder"),
  getPromotionAnalytics: () =>
    apiFetch<PromotionAnalyticsResponse>("/admin/analytics/promotions"),
  getFraudSignals: () => apiFetch<FraudSignal[]>("/admin/analytics/fraud"),

  listProducts: (params?: {
    page?: number;
    page_size?: number;
    search?: string;
    status?: string;
    category?: string;
    warehouse_id?: string;
    /** Sum inventory across active variants (one row per product). */
    inventory?: "product";
  }) => {
    const sp = new URLSearchParams();
    if (params?.page) sp.set("page", String(params.page));
    if (params?.page_size) sp.set("page_size", String(params.page_size));
    if (params?.search) sp.set("search", params.search);
    if (params?.status) sp.set("status", params.status);
    if (params?.category) sp.set("category", params.category);
    if (params?.warehouse_id) sp.set("warehouse_id", params.warehouse_id);
    if (params?.inventory === "product") sp.set("inventory", "product");
    return apiFetch<{ items: ProductListItem[]; total: number }>(
      `/admin/products?${sp.toString()}`,
    );
  },
  createProduct: (payload: Record<string, unknown>) =>
    apiFetch<{ product: Product; variants: unknown[] }>("/api/v1/products/", {
      method: "POST",
      auth: false,
      body: JSON.stringify(payload),
    }),
  getProduct: (id: string) =>
    apiFetch<{ product: Product; variants: unknown[]; collection_ids?: string[] }>(`/api/v1/products/${id}`, {
      auth: false,
    }),
  setPrice: (id: string, payload: ChannelPrice) =>
    apiFetch<ChannelPrice>(`/api/v1/products/${id}/prices`, {
      method: "PUT",
      auth: false,
      body: JSON.stringify(payload),
    }),

  listOrders: (params?: {
    page?: number;
    page_size?: number;
    status?: string;
    channel?: string;
    date_from?: string;
    date_to?: string;
    customer_id?: string;
  }) => {
    const sp = new URLSearchParams();
    if (params?.page) sp.set("page", String(params.page));
    if (params?.page_size) sp.set("page_size", String(params.page_size));
    if (params?.status) sp.set("status", params.status);
    if (params?.channel) sp.set("channel", params.channel);
    if (params?.date_from) sp.set("date_from", params.date_from);
    if (params?.date_to) sp.set("date_to", params.date_to);
    if (params?.customer_id) sp.set("customer_id", params.customer_id);
    return apiFetch<{ items: OrderListItem[]; total: number }>(
      `/admin/orders?${sp.toString()}`,
    );
  },
  createOrder: (payload: Record<string, unknown>) =>
    apiFetch<{ order: Order; fifo_results: unknown[]; invoice?: OrderInvoice }>(
      "/api/v1/orders/",
      {
        method: "POST",
        auth: false,
        body: JSON.stringify(payload),
      },
    ),
  getOrder: (id: string) => apiFetch<Order>(`/api/v1/orders/${id}`, { auth: false }),
  getInvoiceXml: async (id: string) => {
    const response = await fetch(`${getApiBaseUrl()}/api/v1/orders/${id}/invoice`);
    if (!response.ok) {
      const text = await response.text();
      throw new ApiError(response.status, text);
    }
    return response.text();
  },

  listReturns: (params?: {
    page?: number;
    page_size?: number;
    status?: string;
    order_id?: string;
    customer_id?: string;
  }) => {
    const sp = new URLSearchParams();
    if (params?.page) sp.set("page", String(params.page));
    if (params?.page_size) sp.set("page_size", String(params.page_size));
    if (params?.status) sp.set("status", params.status);
    if (params?.order_id) sp.set("order_id", params.order_id);
    if (params?.customer_id) sp.set("customer_id", params.customer_id);
    return apiFetch<{ items: ReturnListItem[]; total: number }>(
      `/admin/returns?${sp.toString()}`,
    );
  },
  getReturn: (id: string) =>
    apiFetch<ReturnRequest>(`/api/v1/returns/${id}`, { auth: false }),

  listShipments: (params?: {
    page?: number;
    page_size?: number;
    status?: string;
    carrier?: string;
    warehouse_id?: string;
  }) => {
    const sp = new URLSearchParams();
    if (params?.page) sp.set("page", String(params.page));
    if (params?.page_size) sp.set("page_size", String(params.page_size));
    if (params?.status) sp.set("status", params.status);
    if (params?.carrier) sp.set("carrier", params.carrier);
    if (params?.warehouse_id) sp.set("warehouse_id", params.warehouse_id);
    return apiFetch<{ items: ShipmentListItem[]; total: number }>(
      `/admin/shipments?${sp.toString()}`,
    );
  },
  createShipment: (payload: Record<string, unknown>) =>
    apiFetch<Shipment>("/admin/shipments/create", {
      method: "POST",
      body: JSON.stringify(payload),
    }),
  getShipment: (id: string) => apiFetch<Shipment>(`/admin/shipments/${id}`),
  getShipmentTracking: (id: string) =>
    apiFetch<ShipmentEvent[]>(`/admin/shipments/${id}/tracking`),

  listActivityLog: (params?: {
    page?: number;
    page_size?: number;
    search?: string;
  }) => {
    const sp = new URLSearchParams();
    if (params?.page) sp.set("page", String(params.page));
    if (params?.page_size) sp.set("page_size", String(params.page_size));
    if (params?.search) sp.set("search", params.search);
    return apiFetch<{ items: ActivityLogItem[]; total: number }>(
      `/admin/activity-log?${sp.toString()}`,
    );
  },

  // ─── Product Module (Shopify-style) ──────────────────────────────────────
  createProductV2: (payload: ProductCreatePayload) =>
    apiFetch<ProductDetail>("/api/v1/products/", {
      method: "POST",
      body: JSON.stringify({
        ...payload,
        name: payload.title,
        track_inventory: payload.track_inventory ?? true,
        warehouse_id: payload.warehouse_id ?? undefined,
        vat_type: payload.vat_type ?? "standard",
        country_of_origin: payload.country_of_origin ?? "AE",
        variants: (payload.variants ?? []).map((v) => ({
          sku: v.sku,
          barcode: v.barcode,
          color: v.options?.["Color"],
          size: v.options?.["Size"],
          weight_g: v.weight_g,
          image_url: v.media?.[0]?.url,
          media_urls: (v.media ?? []).map((m) => m.url).filter(Boolean),
          price: v.price,
          sale_price: v.sale_price,
          cost: v.cost,
          quantity: v.quantity,
        })),
      }),
    }),

  createDraftProduct: () =>
    apiFetch<Product>("/admin/products/drafts", {
      method: "POST",
    }),

  updateProduct: (id: string, payload: Partial<ProductCreatePayload>) =>
    apiFetch<{ status: string }>(`/admin/products/${id}`, {
      method: "PATCH",
      body: JSON.stringify(payload),
    }),

  upsertDefaultVariant: (
    productId: string,
    payload: { sku: string; color?: string; size?: string; image_url?: string },
  ) =>
    apiFetch<Variant>(`/admin/products/${productId}/variants/default`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  createVariant: (
    productId: string,
    payload: {
      sku: string;
      color?: string;
      size?: string;
      image_url?: string | null;
      media_urls?: string[];
      price?: string;
      sale_price?: string;
      cost?: string;
      quantity?: number;
    },
  ) =>
    apiFetch<Variant>(`/admin/products/${productId}/variants`, {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  patchVariant: (
    variantId: string,
    payload: {
      sku: string;
      color?: string;
      size?: string;
      image_url?: string | null;
      media_urls?: string[];
      price?: string;
      sale_price?: string;
      cost?: string;
      quantity?: number;
    },
  ) =>
    apiFetch<{ status: string }>(`/admin/products/variants/${variantId}`, {
      method: "PATCH",
      body: JSON.stringify(payload),
    }),

  deleteVariant: (variantId: string) =>
    apiFetch<{ status: string }>(`/admin/products/variants/${variantId}`, {
      method: "DELETE",
    }),

  duplicateProduct: (id: string) =>
    apiFetch<Product>(`/admin/products/${id}/duplicate`, {
      method: "POST",
    }),

  deleteProduct: (id: string) =>
    apiFetch<void>(`/admin/products/${id}`, { method: "DELETE" }),

  listInventoryRows: (params?: {
    warehouse_id?: string;
    product?: string;
    category?: string;
    low_stock?: boolean;
  }) => {
    const sp = new URLSearchParams();
    if (params?.warehouse_id) sp.set("warehouse_id", params.warehouse_id);
    if (params?.product) sp.set("product", params.product);
    if (params?.category) sp.set("category", params.category);
    if (params?.low_stock) sp.set("low_stock", "true");
    return apiFetch<{ items: InventoryListItem[]; total: number }>(
      `/admin/inventory?${sp.toString()}`,
    );
  },

  adjustInventory: (payload: {
    warehouse_id: string;
    variant_id: string;
    adjustment_type: "increase" | "decrease";
    quantity: number;
    reason?: string;
  }) =>
    apiFetch<void>("/admin/inventory/adjust", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  listTransfers: () =>
    apiFetch<{ items: InventoryTransfer[]; total: number }>("/admin/transfers"),

  getTransfer: (id: string) =>
    apiFetch<InventoryTransfer>(`/admin/transfers/${id}`),

  createTransfer: (payload: {
    reference?: string;
    origin_warehouse_id: string;
    destination_warehouse_id: string;
    notes?: string;
    tags?: string[];
    items: Array<{ id?: string; variant_id: string; quantity: number }>;
    status?: "draft" | "pending" | "in_transit" | "completed" | "cancelled";
  }) =>
    apiFetch<InventoryTransfer>("/admin/transfers", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  updateTransfer: (
    id: string,
    payload: {
      reference?: string;
      origin_warehouse_id: string;
      destination_warehouse_id: string;
      notes?: string;
      tags?: string[];
      items: Array<{ id?: string; variant_id: string; quantity: number }>;
    },
  ) =>
    apiFetch<InventoryTransfer>(`/admin/transfers/${id}`, {
      method: "PATCH",
      body: JSON.stringify(payload),
    }),

  transitionTransfer: (id: string, status: "draft" | "pending" | "in_transit" | "completed" | "cancelled") =>
    apiFetch<InventoryTransfer>(`/admin/transfers/${id}/status`, {
      method: "POST",
      body: JSON.stringify({ status }),
    }),

  listCategories: async () => {
    try {
      return await apiFetch<ProductCategory[]>("/admin/products/categories");
    } catch {
      return [] as ProductCategory[];
    }
  },

  getCategory: (id: string) =>
    apiFetch<ProductCategory>(`/admin/products/categories/${id}`),

  createCategory: (payload: Partial<ProductCategory>) =>
    apiFetch<ProductCategory>("/admin/products/categories", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  updateCategory: (id: string, payload: Partial<ProductCategory>) =>
    apiFetch<ProductCategory>(`/admin/products/categories/${id}`, {
      method: "PATCH",
      body: JSON.stringify(payload),
    }),

  deleteCategory: (id: string) =>
    apiFetch<void>(`/admin/products/categories/${id}`, { method: "DELETE" }),

  listCollections: () => apiFetch<ProductCollection[]>("/admin/collections"),

  getCollection: (id: string) => apiFetch<ProductCollection>(`/admin/collections/${id}`),

  createCollection: (payload: Partial<ProductCollection>) =>
    apiFetch<ProductCollection>("/admin/collections", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  updateCollection: (id: string, payload: Partial<ProductCollection>) =>
    apiFetch<ProductCollection>(`/admin/collections/${id}`, {
      method: "PATCH",
      body: JSON.stringify(payload),
    }),

  deleteCollection: (id: string) =>
    apiFetch<void>(`/admin/collections/${id}`, { method: "DELETE" }),

  // ─── Media Library ───────────────────────────────────────────────────────
  uploadMedia: (file: File) => {
    const formData = new FormData();
    formData.append("file", file);
    return apiFetch<MediaAsset>("/admin/media/upload", {
      method: "POST",
      body: formData,
    });
  },

  listMedia: (params?: {
    limit?: number;
    cursor?: string;
    type?: string;
    search?: string;
  }) => {
    const sp = new URLSearchParams();
    if (params?.limit) sp.set("limit", String(params.limit));
    if (params?.cursor) sp.set("cursor", params.cursor);
    if (params?.type) sp.set("type", params.type);
    if (params?.search) sp.set("search", params.search);
    return apiFetch<{ items: MediaAsset[]; next_cursor: string | null }>(
      `/admin/media?${sp.toString()}`,
    );
  },

  patchMedia: (id: string, payload: { alt?: string; tags?: string[] }) =>
    apiFetch<void>(`/admin/media/${id}`, {
      method: "PATCH",
      body: JSON.stringify(payload),
    }),

  deleteMedia: (id: string) =>
    apiFetch<void>(`/admin/media/${id}`, { method: "DELETE" }),
};
