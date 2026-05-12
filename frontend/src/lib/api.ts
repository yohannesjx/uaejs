import { browser } from "$app/environment";
import { env } from "$env/dynamic/public";
import type { ProductColorSwatch, ProductVariantAvailability, StoreCollection, UiProduct } from "./types";

/**
 * API origin for the browser and for SSR fetches.
 * When the app is opened on a public host but env still points at loopback,
 * use same hostname with the API port (mirrors admin-dashboard api-client).
 */
function getApiBaseUrl(): string {
  const fromEnv = env.PUBLIC_API_BASE_URL;
  const apiPort = env.PUBLIC_API_PORT ?? "8080";

  if (browser) {
    const h = window.location.hostname;
    const onLoopback = h === "localhost" || h === "127.0.0.1";
    if (!onLoopback) {
      const envLooksLoopback =
        !fromEnv || fromEnv.includes("localhost") || fromEnv.includes("127.0.0.1");
      if (envLooksLoopback) {
        return `${window.location.protocol}//${h}:${apiPort}`;
      }
      return fromEnv;
    }
    if (fromEnv) return fromEnv;
    return `http://localhost:${apiPort}`;
  }

  if (fromEnv) return fromEnv;
  return `http://127.0.0.1:${apiPort}`;
}

/**
 * API may store media as http://127.0.0.1:8080/uploads/...; browsers block that from a public storefront.
 * Rebuild the URL using the same origin as {@link getApiBaseUrl}.
 */
export function publicAssetUrl(stored: string | null | undefined): string {
  if (stored == null || stored === "") return "";
  const raw = stored.trim();
  const idx = raw.indexOf("/uploads/");
  if (idx === -1) return raw;
  return `${getApiBaseUrl()}${raw.slice(idx)}`;
}

function rewriteUiProductMedia(p: UiProduct): UiProduct {
  return {
    ...p,
    imageUrl: p.imageUrl ? publicAssetUrl(p.imageUrl) : null,
    images: (p.images ?? []).map((u) => publicAssetUrl(u)),
    colorSwatches: (p.colorSwatches ?? []).map((s) => ({
      ...s,
      imageUrl: s.imageUrl ? publicAssetUrl(s.imageUrl) : null,
      images: (s.images ?? []).map((u) => publicAssetUrl(u)),
    })),
  };
}

const CACHE_TTL_MS = 30_000;
const DEBOUNCE_MS = 120;
const ADMIN_PAGE_SIZE = 100;

type CacheEntry<T> = {
  value: T;
  expiresAt: number;
};

const memoryCache = new Map<string, CacheEntry<unknown>>();
const inFlight = new Map<string, Promise<unknown>>();
const debounced = new Map<string, Promise<unknown>>();

function getCache<T>(key: string): T | null {
  const hit = memoryCache.get(key);
  if (!hit) return null;
  if (Date.now() > hit.expiresAt) {
    memoryCache.delete(key);
    return null;
  }
  return hit.value as T;
}

function getCacheEntry<T>(key: string): CacheEntry<T> | null {
  const hit = memoryCache.get(key);
  if (!hit) return null;
  return hit as CacheEntry<T>;
}

function setCache<T>(key: string, value: T, ttlMs = CACHE_TTL_MS): T {
  memoryCache.set(key, { value, expiresAt: Date.now() + ttlMs });
  return value;
}

function deleteCacheByPrefix(prefix: string) {
  for (const key of memoryCache.keys()) {
    if (key.startsWith(prefix)) memoryCache.delete(key);
  }
}

/** Aligns storefront requests with backend collection_slug sanitization (trim, no leading slashes). */
function normalizeCollectionSlugParam(collectionSlug?: string): string | undefined {
  const s = collectionSlug?.trim().replace(/^\/+|\/+$/g, "").trim();
  return s || undefined;
}

function productsListCacheKey(collectionSlug?: string): string {
  const s = normalizeCollectionSlugParam(collectionSlug);
  return s ? `products:list:${encodeURIComponent(s)}` : "products:list";
}

function isApiStatusError(e: unknown): e is Error & { status: number } {
  return (
    e instanceof Error &&
    "status" in e &&
    typeof (e as { status: unknown }).status === "number"
  );
}

function asRecord(input: unknown): Record<string, unknown> | null {
  return typeof input === "object" && input !== null ? (input as Record<string, unknown>) : null;
}

function asString(v: unknown, fallback = ""): string {
  return typeof v === "string" ? v : fallback;
}

/** Trim variant option keys so storefront filters match admin-entered spacing/casing quirks. */
function asTrimmedString(v: unknown, fallback = ""): string {
  return typeof v === "string" ? v.trim() : fallback;
}

function asNumber(v: unknown, fallback = 0): number {
  return typeof v === "number" && Number.isFinite(v) ? v : fallback;
}

function asNumberLike(v: unknown, fallback = 0): number {
  if (typeof v === "number" && Number.isFinite(v)) return v;
  if (typeof v === "string") {
    const n = Number(v);
    return Number.isFinite(n) ? n : fallback;
  }
  return fallback;
}

function asStringArray(v: unknown): string[] {
  return Array.isArray(v) ? v.filter((x): x is string => typeof x === "string") : [];
}

function compactDistinctStrings(input: string[]): string[] {
  const out: string[] = [];
  for (const raw of input) {
    const val = raw.trim();
    if (!val) continue;
    if (!out.includes(val)) out.push(val);
  }
  return out;
}

function addOrUpdateSwatch(swatches: ProductColorSwatch[], name: string, image: string | null, additionalImages: string[] = []) {
  const n = name.trim();
  if (!n) return;
  const existing = swatches.find((s) => s.name.toLowerCase() === n.toLowerCase());
  const allImgs = compactDistinctStrings([image || "", ...additionalImages]);
  if (existing) {
    if (!existing.imageUrl && image) existing.imageUrl = image;
    for (const img of allImgs) {
      if (!existing.images.includes(img)) existing.images.push(img);
    }
    return;
  }
  swatches.push({ name: n, imageUrl: image || null, images: allImgs });
}

function getCookieValue(name: string): string | null {
  if (typeof document === "undefined") return null;
  const prefix = `${encodeURIComponent(name)}=`;
  const part = document.cookie.split(";").map((p) => p.trim()).find((p) => p.startsWith(prefix));
  if (!part) return null;
  const value = part.slice(prefix.length);
  return value ? decodeURIComponent(value) : null;
}

export function validateProduct(raw: unknown): raw is Record<string, unknown> {
  const r = asRecord(raw);
  if (!r) return false;
  const id = asString(r.id || r.product_id);
  const title = asString(r.title || r.name);
  return Boolean(id && title);
}

function normalizeProduct(raw: unknown): UiProduct | null {
  if (!validateProduct(raw)) return null;
  const r = asRecord(raw);
  if (!r) return null;
  const id = asString(r.id || r.product_id);
  const slug = asString(r.slug || r.handle || id);
  const title = asString(r.title || r.name);
  if (!id || !title) return null;

  return rewriteUiProductMedia({
    id,
    slug: slug || id,
    title,
    price: asNumberLike(r.price, 0),
    compareAtPrice: r.compare_at_price === null ? null : asNumberLike(r.compare_at_price, 0) || null,
    imageUrl: asString(r.image_url || r.thumbnail) || null,
    images: asStringArray(r.images),
    category: asString(r.category, "uncategorized"),
    color: asString(r.color, "Unknown"),
    colors: compactDistinctStrings([asString(r.color)]),
    colorSwatches: compactDistinctStrings([asString(r.color)]).map((name) => ({ name, imageUrl: null, images: [] })),
    sizeOptions: asStringArray(r.sizes).length ? asStringArray(r.sizes).map((s) => s.trim()).filter(Boolean) : [],
    tags: asStringArray(r.tags),
    description: asString(r.description, ""),
    inventory: r.inventory === null ? null : asNumber(r.inventory, 0),
    variants: []
  });
}

function normalizeAdminProductList(payload: unknown): UiProduct[] {
  const obj = asRecord(payload);
  const rawItems = Array.isArray(obj?.items) ? obj.items : [];
  const grouped = new Map<string, UiProduct>();

  for (const raw of rawItems) {
    const row = asRecord(raw);
    if (!row) continue;
    const status = asString(row.status).toLowerCase();
    if (status && status !== "active" && status !== "on") continue;
    const productID = asString(row.product_id || row.id);
    const variantID = asString(row.id);
    const name = asString(row.name, "Untitled");
    if (!productID || !variantID || !name) continue;

    const existing = grouped.get(productID);
    const price = asNumberLike(row.price, 0);
    const thumbnail = asString(row.thumbnail);
    const size = asTrimmedString(row.size);
    const color = asTrimmedString(row.color);
    const stock = asNumberLike(row.stock, 0);
    const slug = asString(row.slug, productID);
    const category = asString(row.category, "uncategorized");

    if (!existing) {
      grouped.set(productID, {
        id: productID,
        slug: slug || productID,
        title: name,
        price,
        compareAtPrice: null,
        imageUrl: thumbnail || null,
        images: thumbnail ? [thumbnail] : [],
        category,
        color: color || "",
        colors: compactDistinctStrings(color ? [color] : []),
        colorSwatches: color ? [{ name: color, imageUrl: thumbnail || null, images: thumbnail ? [thumbnail] : [] }] : [],
        sizeOptions: size ? [size] : [],
        tags: [],
        description: "",
        inventory: stock,
        variants: [{
          size: size || "",
          color: color || "",
          stock,
          isAvailable: stock > 0
        }]
      });
      continue;
    }

    if (price > 0 && (existing.price <= 0 || price < existing.price)) {
      existing.price = price;
    }
    if (!existing.imageUrl && thumbnail) existing.imageUrl = thumbnail;
    if (thumbnail && !existing.images.includes(thumbnail)) existing.images.push(thumbnail);
    if (size && !existing.sizeOptions.includes(size)) existing.sizeOptions.push(size);
    if (!asTrimmedString(existing.color) && color) existing.color = color;
    if (color && !existing.colors.includes(color)) existing.colors.push(color);
    if (color) addOrUpdateSwatch(existing.colorSwatches, color, thumbnail || null);
    if (existing.inventory === null) existing.inventory = stock;
    else existing.inventory += stock;
    existing.variants.push({
      size: size || "",
      color: color || "",
      stock,
      isAvailable: stock > 0
    });
  }

  return Array.from(grouped.values()).map(rewriteUiProductMedia);
}

function normalizeProductDetail(raw: unknown, fallback: UiProduct | null = null): UiProduct | null {
  const obj = asRecord(raw);
  if (!obj) return fallback;
  const product = asRecord(obj.product);
  const variants = Array.isArray(obj.variants) ? obj.variants : [];
  if (!product) return fallback;

  const productID = asString(product.id, fallback?.id ?? "");
  if (!productID) return fallback;

  const title = asString(product.name, fallback?.title ?? "");
  const slug = asString(product.slug, fallback?.slug ?? productID);
  const category = asString(product.category, fallback?.category ?? "uncategorized");
  const description = asString(product.description, fallback?.description ?? "");

  const images = new Set<string>(fallback?.images ?? []);
  const sizes = new Set<string>((fallback?.sizeOptions ?? []).map((s) => s.trim()).filter(Boolean));
  const colors = new Set<string>((fallback?.colors ?? []).map((s) => s.trim()).filter(Boolean));
  const swatches: ProductColorSwatch[] = (fallback?.colorSwatches ?? []).map((s) => ({ ...s }));
  let price = fallback?.price ?? 0;
  let compareAtPrice: number | null = fallback?.compareAtPrice ?? null;
  let inventory = fallback?.inventory ?? 0;
  const variantsAvailability: ProductVariantAvailability[] = [];

  for (const v of variants) {
    const vr = asRecord(v);
    if (!vr) continue;
    const image = asString(vr.image_url);
    const size = asTrimmedString(vr.size);
    const color = asTrimmedString(vr.color);
    const qty = asNumberLike(vr.quantity, 0);
    const salePrice = asNumberLike(vr.sale_price, 0);
    const basePrice = asNumberLike(vr.price, 0);
    const variantImages = asStringArray(vr.media_urls);
    if (image) images.add(image);
    for (const vi of variantImages) images.add(vi);

    if (size) sizes.add(size);
    if (color) {
      colors.add(color);
      addOrUpdateSwatch(swatches, color, image || null, variantImages);
    }
    inventory += qty;
    variantsAvailability.push({
      size: size || "",
      color: color || "",
      stock: qty,
      isAvailable: qty > 0
    });
    if (salePrice > 0) {
      if (price <= 0 || salePrice < price) price = salePrice;
      if (basePrice > salePrice && (compareAtPrice === null || basePrice > compareAtPrice)) compareAtPrice = basePrice;
    } else if (basePrice > 0 && (price <= 0 || basePrice < price)) {
      price = basePrice;
    }
  }

  const imageList = Array.from(images);
  const colorList = colors.size > 0 ? Array.from(colors) : [];
  const fallbackColors = (fallback?.colors ?? [])
    .map((c) => asTrimmedString(c))
    .filter((c) => c.length > 0 && c.toLowerCase() !== "unknown");
  /** Do not use "Unknown" when no variant has a color — it breaks PDP stock filters (variant color is ""). */
  const primaryColor = colorList[0] ?? fallbackColors[0] ?? "";

  return rewriteUiProductMedia({
    id: productID,
    slug: slug || productID,
    title: title || fallback?.title || "Untitled",
    price,
    compareAtPrice,
    imageUrl: imageList[0] ?? fallback?.imageUrl ?? null,
    images: imageList,
    category,
    color: primaryColor,
    colors: colorList.length > 0 ? colorList : fallbackColors,
    colorSwatches: swatches,
    sizeOptions:
      sizes.size > 0
        ? Array.from(sizes)
        : (() => {
            const fb = (fallback?.sizeOptions ?? []).map((s) => s.trim()).filter(Boolean);
            if (fb.length > 0) return fb;
            return variantsAvailability.length > 0 ? [""] : [];
          })(),
    tags: fallback?.tags ?? [],
    description,
    inventory,
    variants: variantsAvailability
  });
}

function buildHeaders(): Headers {
  const headers = new Headers();
  headers.set("Accept", "application/json");
  if (typeof window !== "undefined") {
    const token =
      window.localStorage.getItem("dubai-admin-access-token") ??
      getCookieValue("access_token");
    const tenantID = window.localStorage.getItem("dubai-admin-tenant-id");
    if (token) headers.set("Authorization", `Bearer ${token}`);
    if (tenantID) headers.set("X-Tenant-ID", tenantID);
  }
  return headers;
}

async function requestUnknown(path: string): Promise<unknown> {
  const key = `request:${path}`;
  const existing = inFlight.get(key);
  if (existing) return existing;

  const run = (async () => {
    const res = await fetch(`${getApiBaseUrl()}${path}`, { headers: buildHeaders(), credentials: "include" });
    if (!res.ok) throw new Error(`API ${res.status}`);
    return res.json();
  })();
  inFlight.set(key, run);
  try {
    return await run;
  } finally {
    inFlight.delete(key);
  }
}

function debouncedCall<T>(key: string, fn: () => Promise<T>): Promise<T> {
  const existing = debounced.get(key) as Promise<T> | undefined;
  if (existing) return existing;
  const p = new Promise<T>((resolve, reject) => {
    const t = setTimeout(async () => {
      try {
        resolve(await fn());
      } catch (e) {
        reject(e);
      } finally {
        debounced.delete(key);
      }
      clearTimeout(t);
    }, DEBOUNCE_MS);
  });
  debounced.set(key, p);
  return p;
}

async function fetchAllStoreProductsAggregate(collectionSlug?: string): Promise<{ items: unknown[]; total: number }> {
  let page = 1;
  let total = 0;
  const aggregate: unknown[] = [];

  do {
    const qs = new URLSearchParams({
      page: String(page),
      page_size: String(ADMIN_PAGE_SIZE),
    });
    const slug = normalizeCollectionSlugParam(collectionSlug);
    if (slug) qs.set("collection_slug", slug);

    const res = await fetch(`${getApiBaseUrl()}/store/products?${qs}`, {
      headers: buildHeaders(),
      credentials: "include",
    });
    if (!res.ok) {
      const err = new Error(`API ${res.status}`) as Error & { status: number };
      err.status = res.status;
      throw err;
    }
    const p: unknown = await res.json();
    const obj = asRecord(p);
    const items = Array.isArray(obj?.items) ? obj.items : [];
    total = asNumberLike(obj?.total, items.length);
    aggregate.push(...items);
    page += 1;
    if (!items.length) break;
  } while (aggregate.length < total);

  return { items: aggregate, total: aggregate.length };
}

async function cachedProductsRequest(collectionSlug?: string): Promise<UiProduct[]> {
  const cacheKey = productsListCacheKey(collectionSlug);
  const cached = getCache<UiProduct[]>(cacheKey);
  if (cached) return cached;
  const payload = await debouncedCall(cacheKey, async () => fetchAllStoreProductsAggregate(collectionSlug));
  const normalized = normalizeAdminProductList(payload);
  return setCache(cacheKey, normalized);
}

async function cachedProductRequest(handleOrId: string): Promise<UiProduct | null> {
  const cacheKey = `products:one:${handleOrId}`;
  const cached = getCache<UiProduct | null>(cacheKey);
  if (cached !== null) return cached;
  const list = await getProducts(undefined);
  const fromList = list.find((p) => p.slug === handleOrId || p.id === handleOrId) ?? null;
  if (!fromList) return setCache(cacheKey, null, CACHE_TTL_MS / 2);
  const payload = await debouncedCall(cacheKey, () =>
    requestUnknown(`/api/v1/products/${encodeURIComponent(fromList.id)}`)
  );
  const normalized = normalizeProductDetail(payload, fromList);
  return setCache(cacheKey, normalized, CACHE_TTL_MS / 2);
}

function revalidateProductsInBackground(collectionSlug?: string) {
  const key = productsListCacheKey(collectionSlug);
  void debouncedCall(`revalidate:${key}`, async () => fetchAllStoreProductsAggregate(collectionSlug))
    .then((payload) => {
      const normalized = normalizeAdminProductList(payload);
      setCache(key, normalized);
    })
    .catch(() => {
      // preserve stale cache on background revalidation failure
    });
}

function revalidateProductInBackground(handleOrId: string) {
  const key = `products:one:${handleOrId}`;
  void getProducts(undefined)
    .then((list) => {
      const base = list.find((p) => p.slug === handleOrId || p.id === handleOrId) ?? null;
      if (!base) {
        setCache(key, null, CACHE_TTL_MS / 2);
        return null;
      }
      return debouncedCall(key, () => requestUnknown(`/api/v1/products/${encodeURIComponent(base.id)}`)).then((payload) =>
        normalizeProductDetail(payload, base)
      );
    })
    .then((normalized) => {
      if (normalized !== null) setCache(key, normalized, CACHE_TTL_MS / 2);
    })
    .catch(() => {
      // preserve stale cache on background revalidation failure
    });
}

export function clearApiCache() {
  memoryCache.clear();
}

export function invalidateProductsCache(collectionSlug?: string) {
  memoryCache.delete(productsListCacheKey(collectionSlug));
}

export function invalidateProductCache(handleOrId: string) {
  memoryCache.delete(`products:one:${handleOrId}`);
}

export function invalidateAllProductCaches() {
  deleteCacheByPrefix("products:list");
  deleteCacheByPrefix("products:one:");
}

async function requestUnknownNoCache(path: string): Promise<unknown> {
  const res = await fetch(`${getApiBaseUrl()}${path}`, { headers: buildHeaders(), credentials: "include" });
  if (!res.ok) throw new Error(`API ${res.status}`);
  return res.json();
}

export async function getProducts(collectionSlug?: string): Promise<UiProduct[]> {
  // SWR: serve stale cache immediately and revalidate in background.
  const cacheKey = productsListCacheKey(collectionSlug);
  const cachedEntry = getCacheEntry<UiProduct[]>(cacheKey);
  if (cachedEntry) {
    if (Date.now() > cachedEntry.expiresAt) revalidateProductsInBackground(collectionSlug);
    return cachedEntry.value;
  }
  try {
    return await cachedProductsRequest(collectionSlug);
  } catch (e) {
    const slug = normalizeCollectionSlugParam(collectionSlug);
    if (slug && isApiStatusError(e) && e.status === 404) throw e;
    return [];
  }
}

export async function getProduct(handleOrId: string): Promise<UiProduct | null> {
  // SWR for single product as well.
  const cacheKey = `products:one:${handleOrId}`;
  const cachedEntry = getCacheEntry<UiProduct | null>(cacheKey);
  if (cachedEntry) {
    if (Date.now() > cachedEntry.expiresAt) revalidateProductInBackground(handleOrId);
    return cachedEntry.value;
  }
  try {
    return await cachedProductRequest(handleOrId);
  } catch {
    const list = await getProducts(undefined);
    return list.find((p) => p.slug === handleOrId || p.id === handleOrId) ?? null;
  }
}

export async function getRelatedProducts(seedId: string): Promise<UiProduct[]> {
  const list = await getProducts(undefined);
  return list.filter((p) => p.id !== seedId).slice(0, 8);
}

function normalizeStoreCollections(raw: unknown): StoreCollection[] {
  if (!Array.isArray(raw)) return [];
  const out: StoreCollection[] = [];
  for (const row of raw) {
    const o = asRecord(row);
    if (!o) continue;
    const id = asString(o.id);
    const slug = asString(o.slug);
    const title = asString(o.title);
    if (!id || !slug || !title) continue;
    out.push({
      id,
      title,
      slug,
      description: typeof o.description === "string" || o.description === null ? (o.description as string | null) : undefined,
      image_url:
        typeof o.image_url === "string"
          ? publicAssetUrl(o.image_url as string)
          : o.image_url === null
            ? null
            : undefined,
      product_count: asNumberLike(o.product_count, 0),
    });
  }
  return out;
}

export async function listStoreCollections(): Promise<StoreCollection[]> {
  const cacheKey = "collections:store";
  const cached = getCache<StoreCollection[]>(cacheKey);
  if (cached) return cached;
  try {
    const raw = await requestUnknown("/store/collections");
    return setCache(cacheKey, normalizeStoreCollections(raw));
  } catch {
    return [];
  }
}

export async function validateCart(): Promise<unknown> {
  try {
    const result = await requestUnknownNoCache("/cart/validate");
    // Cart validation can affect availability/price, so trigger safe cache invalidation.
    invalidateAllProductCaches();
    return result;
  } catch {
    return null;
  }
}
