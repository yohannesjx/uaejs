import { browser } from "$app/environment";
import type { CartItem, CartItemSnapshot } from "$lib/types";

const STORAGE_KEY = "fashion_cart";
const SCHEMA_VERSION = 1;

/** Empty / whitespace-only dimensions collapse so line keys stay stable (prevents duplicate lines ×2 stock). */
function normalizeSnapshotDims(s: CartItemSnapshot): CartItemSnapshot {
  const trimOrUndef = (v: string | undefined | null) => {
    const t = (v ?? "").trim();
    return t.length === 0 ? undefined : t;
  };
  return {
    ...s,
    size: trimOrUndef(s.size),
    color: trimOrUndef(s.color),
  };
}

function lineKey(snapshot: CartItemSnapshot): string {
  const s = normalizeSnapshotDims(snapshot);
  return `${s.id}:${s.size ?? "-"}:${s.color ?? "-"}`;
}

type CartPersistedV1 = {
  version: 1;
  items: CartItem[];
};

function isCartItem(input: unknown): input is CartItem {
  if (typeof input !== "object" || input === null) return false;
  const r = input as Record<string, unknown>;
  return typeof r.key === "string" && typeof r.quantity === "number" && typeof r.snapshot === "object" && r.snapshot !== null;
}

/** Merge legacy rows that differ only by empty string vs missing size/color. */
function dedupeCartItems(items: CartItem[]): CartItem[] {
  const map = new Map<string, CartItem>();
  for (const it of items) {
    if (!isCartItem(it)) continue;
    const snap = normalizeSnapshotDims(it.snapshot);
    const key = lineKey(snap);
    const prev = map.get(key);
    if (!prev) {
      map.set(key, { key, quantity: it.quantity, snapshot: { ...snap, maxQuantity: snap.maxQuantity } });
      continue;
    }
    const maxA = prev.snapshot.maxQuantity ?? null;
    const maxB = snap.maxQuantity ?? null;
    const cap =
      maxA != null && maxB != null ? Math.min(maxA, maxB) : (maxA ?? maxB ?? null);
    const next = prev.quantity + it.quantity;
    prev.quantity = cap != null ? Math.min(cap, next) : next;
    prev.snapshot = {
      ...prev.snapshot,
      ...snap,
      maxQuantity: cap ?? snap.maxQuantity ?? prev.snapshot.maxQuantity,
    };
  }
  return Array.from(map.values());
}

function fromStorage(): CartItem[] {
  if (!browser) return [];
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as unknown;
    let items: CartItem[] = [];
    if (Array.isArray(parsed)) {
      items = parsed.filter(isCartItem);
    } else if (typeof parsed === "object" && parsed !== null) {
      const p = parsed as Partial<CartPersistedV1>;
      if (p.version === SCHEMA_VERSION && Array.isArray(p.items)) {
        items = p.items.filter(isCartItem);
      }
    }
    return dedupeCartItems(items);
  } catch {
    return [];
  }
}

class CartState {
  items = $state<CartItem[]>(fromStorage());
  totalItems = $derived(this.items.reduce((acc, it) => acc + it.quantity, 0));
  totalPrice = $derived(this.items.reduce((acc, it) => acc + it.snapshot.price * it.quantity, 0));

  constructor() {
    // $effect.root() is required here because CartState is instantiated at module
    // level (outside any Svelte component). Using plain $effect() would throw
    // effect_orphan and crash the entire Svelte reactivity system app-wide.
    $effect.root(() => {
      $effect(() => {
        if (!browser) return;
        const payload: CartPersistedV1 = {
          version: SCHEMA_VERSION,
          items: this.items.filter((x) => Number.isFinite(x.quantity) && x.quantity > 0)
        };
        localStorage.setItem(STORAGE_KEY, JSON.stringify(payload));
      });
    });
  }

  add(snapshot: CartItemSnapshot, quantity = 1) {
    const snap = normalizeSnapshotDims(snapshot);
    const key = lineKey(snap);
    const max = snap.maxQuantity ?? null;
    if (max !== null && max <= 0) return;
    const existing = this.items.find((i) => i.key === key);
    if (existing) {
      const cap =
        max != null && existing.snapshot.maxQuantity != null
          ? Math.min(max, existing.snapshot.maxQuantity)
          : (max ?? existing.snapshot.maxQuantity ?? null);
      const next = existing.quantity + quantity;
      existing.quantity = cap != null ? Math.min(cap, next) : next;
      existing.snapshot = {
        ...existing.snapshot,
        ...snap,
        maxQuantity: cap ?? snap.maxQuantity ?? existing.snapshot.maxQuantity,
      };
      existing.key = key;
      return;
    }
    const initialQty = max !== null ? Math.min(max, quantity) : quantity;
    if (initialQty > 0) this.items.push({ key, quantity: initialQty, snapshot: snap });
  }

  remove(key: string) {
    this.items = this.items.filter((i) => i.key !== key);
  }

  updateQty(key: string, qty: number) {
    const item = this.items.find((i) => i.key === key);
    if (!item) return;
    const max = item.snapshot.maxQuantity ?? null;
    if (qty <= 0) {
      this.remove(key);
      return;
    }
    item.quantity = max !== null ? Math.min(max, qty) : qty;
  }
}

export const cart = new CartState();
