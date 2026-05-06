export type UUID = string;
export type ISODateTime = string;
export type DecimalString = string;

export type Permission =
  | "users.manage"
  | "inventory.manage"
  | "analytics.view"
  | "suppliers.manage"
  | "channels.manage"
  | "invoices.sandbox"
  | "orders.manage"
  | "products.read"
  | "products.write"
  | "pricing.manage"
  | "returns.approve"
  | string;

export interface ApiErrorPayload {
  error?: string;
  message?: string;
  required_permission?: string;
}

export interface AuthTokenPair {
  access_token: string;
  refresh_token: string;
  expires_at: ISODateTime;
}

export interface User {
  id: UUID;
  email: string;
  full_name: string;
  is_active: boolean;
  roles?: Array<{ id: UUID; name: string; description?: string }>;
  permissions?: Permission[];
  permissions_version?: number;
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
}

export interface Tenant {
  id: UUID;
  name: string;
  domain?: string | null;
  plan?: string;
  is_active?: boolean;
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
}

export interface TenantSettings {
  tenant_id: UUID;
  settings: Record<string, unknown>;
  updated_at?: ISODateTime;
}

export interface Warehouse {
  id: UUID;
  tenant_id: UUID;
  name: string;
  type: "warehouse" | "store" | "dropship" | "virtual";
  address: string;
  city: string;
  country: string;
  is_active: boolean;
  priority: number;
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
}

export interface WarehouseStock {
  id: UUID;
  warehouse_id: UUID;
  variant_id: UUID;
  qty_on_hand: number;
  qty_reserved: number;
  qty_available: number;
  reorder_point: number;
  reorder_qty: number;
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
}

export interface InventoryMovement {
  id?: UUID;
  variant_id: UUID;
  batch_item_id?: UUID | null;
  order_id?: UUID | null;
  reservation_id?: UUID | null;
  movement_type:
  | "purchase_in"
  | "sale_out"
  | "adjustment_in"
  | "adjustment_out"
  | "reservation"
  | "reservation_release"
  | "return_in"
  | "transfer_in"
  | "transfer_out";
  quantity: number;
  quantity_before: number;
  quantity_after: number;
  unit_cost_snapshot?: DecimalString | null;
  channel_id?: UUID | null;
  reference?: string | null;
  notes?: string | null;
  created_at?: ISODateTime;
}

export interface TransferResult {
  from_stock: WarehouseStock;
  to_stock: WarehouseStock;
  movements: InventoryMovement[];
}

export interface Customer {
  id: UUID;
  tenant_id: UUID;
  email: string;
  phone?: string | null;
  full_name: string;
  loyalty_tier: "bronze" | "silver" | "gold" | "vip";
  is_active: boolean;
  notes?: string | null;
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
}

export interface LoyaltyAccount {
  id: UUID;
  customer_id: UUID;
  points_balance: number;
  lifetime_points: number;
  updated_at?: ISODateTime;
}

export interface LoyaltyTransaction {
  id: UUID;
  account_id: UUID;
  order_id?: UUID | null;
  tx_type: "earned" | "redeemed" | "expired" | "adjusted" | "refunded";
  points: number;
  balance_before: number;
  balance_after: number;
  note?: string | null;
  created_at?: ISODateTime;
}

export interface CustomerProfileResponse {
  customer: Customer;
  loyalty_account?: LoyaltyAccount | null;
}

export interface PointsRedemptionResult {
  points_redeemed: number;
  discount_aed: DecimalString;
  balance_after: number;
}

export interface Supplier {
  id: UUID;
  name: string;
  contact_name?: string;
  phone?: string;
  email?: string;
  country?: string;
  lead_time_days?: number;
  minimum_order_qty?: number;
  rating?: DecimalString;
  notes?: string;
  is_active?: boolean;
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
}

export interface ExternalPlatform {
  id: UUID;
  name: string;
  type: "shopify" | "amazon" | "instagram" | "tiktok" | "noon" | string;
  is_active: boolean;
  created_at?: ISODateTime;
}

export interface ShippingAddress {
  line1: string;
  line2?: string;
  city: string;
  emirate: string;
  country: string;
  postal_code?: string;
}

export interface ShipmentEvent {
  id: UUID;
  shipment_id: UUID;
  status: string;
  location?: string;
  description?: string;
  event_time?: ISODateTime;
  recorded_at?: ISODateTime;
}

export interface Shipment {
  id: UUID;
  order_id: UUID;
  account_id?: UUID | null;
  tracking_number?: string | null;
  carrier_ref?: string | null;
  status: string;
  weight_g?: number;
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
  events?: ShipmentEvent[];
}

export interface ReturnItem {
  id: UUID;
  return_id: UUID;
  order_item_id: UUID;
  variant_id: UUID;
  batch_item_id?: UUID | null;
  quantity: number;
  condition: "good" | "damaged" | "wrong_item";
  qc_photo_hash_customer?: string | null;
  qc_photo_hash_outbound?: string | null;
  qc_match_score?: DecimalString | null;
  qc_passed?: boolean | null;
  qc_reviewed_at?: ISODateTime | null;
  qc_reviewer_notes?: string | null;
  cogs_per_unit_reversed?: DecimalString | null;
}

export interface ReturnRequest {
  id: UUID;
  order_id: UUID;
  status: string;
  customer_name: string;
  customer_email: string;
  return_reason: string;
  rejection_reason?: string | null;
  requested_at?: ISODateTime;
  received_at?: ISODateTime | null;
  resolved_at?: ISODateTime | null;
  notes?: string | null;
  items?: ReturnItem[];
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
}

export interface ForecastResult {
  sku: string;
  variant_id: UUID;
  channel: string;
  current_stock: number;
  weekly_forecast_units: DecimalString;
  daily_forecast_units: DecimalString;
  days_of_stock_left: DecimalString;
  reorder_suggested: boolean;
  confidence: DecimalString;
  weeks_of_history: number;
  algorithm: string;
}

export interface ReorderSuggestion {
  variant_id: UUID;
  sku: string;
  product_name: string;
  current_stock: number;
  weekly_forecast_units: DecimalString;
  days_of_stock_left: DecimalString;
  suggested_order_qty: number;
  priority: string;
}

export interface FraudSignal {
  customer_email: string;
  return_rate: DecimalString;
  qc_mismatches: number;
  total_returns: number;
  risk_level: string;
  reason: string;
}

export interface PromotionInsight {
  PromotionID?: UUID;
  VariantID?: UUID;
  SKU?: string;
  Channel?: string;
  PromoPrice?: DecimalString;
  StandardPrice?: DecimalString;
  HitCount?: number;
  TotalRevenue?: DecimalString;
  AvgDiscount?: DecimalString;
  EffectiveFrom?: ISODateTime;
  EffectiveUntil?: ISODateTime;
  revenue_lift?: DecimalString;
  discount_depth?: DecimalString;
  verdict?: string;
}

export interface PromotionAnalyticsResponse {
  generated_at: ISODateTime;
  promotions: PromotionInsight[];
}

export interface Product {
  id: UUID;
  name?: string | null;
  name_ar?: string | null;
  description?: string | null;
  brand?: string | null;
  category?: string | null;
  sub_category?: string | null;
  status: ProductStatus;
  vat_type?: string;
  hs_code?: string | null;
  country_of_origin?: string | null;
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
}

export interface Variant {
  id: UUID;
  product_id: UUID;
  sku: string;
  barcode?: string | null;
  color?: string | null;
  size?: string | null;
  weight_g?: number | null;
  image_url?: string | null;
  media_urls?: string[];
  is_active?: boolean;
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
}

export interface ChannelPrice {
  id?: UUID;
  variant_id: UUID;
  channel_id: UUID;
  price: DecimalString;
  currency: string;
  is_active?: boolean;
  effective_from?: ISODateTime;
  effective_until?: ISODateTime | null;
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
}

export interface OrderItem {
  id: UUID;
  order_id: UUID;
  variant_id: UUID;
  quantity: number;
  unit_price: DecimalString;
  discount_amount: DecimalString;
  vat_rate: DecimalString;
  vat_amount: DecimalString;
  line_total: DecimalString;
  cogs_per_unit?: DecimalString | null;
  total_cogs?: DecimalString | null;
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
}

export interface OrderInvoice {
  id: UUID;
  order_id: UUID;
  invoice_type: "einvoice_ubl" | "receipt";
  invoice_number: string;
  xml_content?: string | null;
  exchange_rate_to_aed: DecimalString;
  trigger_reason: string;
  issued_at?: ISODateTime;
  created_at?: ISODateTime;
}

export interface Order {
  id: UUID;
  channel_id: UUID;
  customer_id?: UUID | null;
  customer_name?: string | null;
  customer_email?: string | null;
  customer_phone?: string | null;
  customer_trn?: string | null;
  subtotal: DecimalString;
  discount_amount: DecimalString;
  vat_amount: DecimalString;
  total_amount: DecimalString;
  currency: string;
  vat_type: string;
  invoice_number?: string | null;
  invoice_issued_at?: ISODateTime | null;
  status: string;
  payment_status: string;
  notes?: string | null;
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
  items?: OrderItem[];
}

export interface DashboardStat {
  label: string;
  value: string;
  delta?: string;
  tone?: "default" | "positive" | "warning" | "danger";
}

export interface PageResponse<T> {
  items: T[];
  total: number;
}

export interface ProductListItem {
  id: UUID;
  product_id: UUID;
  name: string;
  slug: string;
  sku: string;
  category?: string | null;
  thumbnail?: string | null;
  price: DecimalString;
  stock: number;
  status: string;
  created_at?: ISODateTime;
}

export interface OrderListItem {
  id: UUID;
  channel_id: UUID;
  channel_name: string;
  channel_type: string;
  customer_id?: UUID | null;
  customer_name?: string | null;
  customer_email?: string | null;
  total_amount: DecimalString;
  currency: string;
  status: string;
  payment_status: string;
  created_at?: ISODateTime;
}

export interface CustomerListItem {
  id: UUID;
  email: string;
  full_name: string;
  phone?: string | null;
  loyalty_tier: string;
  points_balance: number;
  lifetime_points: number;
  is_active: boolean;
  created_at?: ISODateTime;
}

export interface ReturnListItem {
  id: UUID;
  order_id: UUID;
  customer_id?: UUID | null;
  customer_name: string;
  customer_email: string;
  status: string;
  return_reason: string;
  item_count: number;
  requested_at?: ISODateTime;
}

export interface ShipmentListItem {
  id: UUID;
  order_id: UUID;
  account_id?: UUID | null;
  tracking_number?: string | null;
  carrier_ref?: string | null;
  status: string;
  carrier?: string | null;
  warehouse_id?: UUID | null;
  created_at?: ISODateTime;
}

export interface ActivityLogItem {
  id: string;
  event_type: string;
  title: string;
  description: string;
  actor: string;
  subject_id: string;
  subject_type: string;
  created_at: ISODateTime;
  metadata?: Record<string, unknown>;
}

// ─── Product Module (Shopify-style) ──────────────────────────────────────────

export type ProductStatus = "draft" | "active" | "archived";

export interface ProductCategory {
  id: UUID;
  tenant_id?: UUID;
  title: string;
  slug: string;
  description?: string | null;
  type: "manual" | "smart";
  image_url?: string | null;
  product_count?: number;
  product_ids?: string[];
  conditions?: SmartCollectionCondition[];
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
}

/** Manual-only merchandising group (not a category). */
export interface ProductCollection {
  id: UUID;
  tenant_id?: UUID;
  title: string;
  slug: string;
  description?: string | null;
  image_url?: string | null;
  product_count?: number;
  product_ids?: string[];
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
}

export interface SmartCollectionCondition {
  field: "price" | "tag" | "inventory" | "title";
  operator: "less_than" | "greater_than" | "equals" | "contains";
  value: string;
}

export interface ProductOption {
  name: string;
  values: string[];
}

export interface ProductVariantDraft {
  id?: UUID;
  sku: string;
  barcode?: string;
  price: string;
  sale_price?: string;
  cost: string;
  weight_g?: number;
  quantity?: number;
  options: Record<string, string>; // e.g. { Size: "M", Color: "Red" }
  media?: MediaAsset[];
}

export interface ProductCreatePayload {
  title: string;
  slug: string;
  description?: string;
  status: ProductStatus;
  category_id?: UUID | null;
  track_inventory?: boolean;
  warehouse_id?: UUID | null;
  tags?: string[];
  seo_title?: string;
  seo_description?: string;
  weight_g?: number;
  options?: ProductOption[];
  variants?: ProductVariantDraft[];
  // legacy compat
  name?: string;
  brand?: string;
  category?: string;
  vat_type?: string;
  country_of_origin?: string;
  collection_ids?: UUID[];
}

export interface InventoryListItem {
  product_id: UUID;
  product_name: string;
  variant_id: UUID;
  variant_name: string;
  sku: string;
  category: string;
  warehouse_id: UUID;
  warehouse: string;
  available_quantity: number;
  reserved_quantity: number;
  incoming_quantity: number;
}

export type TransferStatus = "draft" | "pending" | "in_transit" | "completed" | "cancelled";

export interface TransferItem {
  id: UUID;
  transfer_id: UUID;
  variant_id: UUID;
  quantity: number;
}

export interface InventoryTransfer {
  id: UUID;
  tenant_id: UUID;
  reference: string;
  origin_warehouse_id: UUID;
  destination_warehouse_id: UUID;
  status: TransferStatus;
  notes?: string | null;
  tags?: string[];
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
  total_items?: number;
  items?: TransferItem[];
}

export interface ProductDetail {
  product: Product & {
    title?: string;
    slug?: string;
    status?: ProductStatus;
    tags?: string[];
    seo_title?: string;
    seo_description?: string;
    options?: ProductOption[];
  };
  variants: (Variant & {
    price?: DecimalString;
    sale_price?: DecimalString;
    cost?: DecimalString;
    options?: Record<string, string>;
  })[];
}

export interface MediaAsset {
  id: UUID;
  tenant_id?: UUID;
  url: string;
  mime_type: string;
  alt?: string;
  tags?: string[];
  sort_order: number;
  size_bytes?: number;
  created_at?: ISODateTime;
  updated_at?: ISODateTime;
}

// Local-only (pre-upload) media item
export interface LocalMediaItem {
  localId: string;
  file?: File;
  previewUrl: string;
  alt?: string;
  uploaded?: boolean;
  remoteId?: UUID;
}

