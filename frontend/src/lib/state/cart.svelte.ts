import { browser } from "$app/environment";
import type { CartItem, CartItemSnapshot } from "$lib/types";

const STORAGE_KEY = "fashion_cart";
const SCHEMA_VERSION = 1;

type CartPersistedV1 = {
  version: 1;
  items: CartItem[];
};

function isCartItem(input: unknown): input is CartItem {
  if (typeof input !== "object" || input === null) return false;
  const r = input as Record<string, unknown>;
  return typeof r.key === "string" && typeof r.quantity === "number" && typeof r.snapshot === "object" && r.snapshot !== null;
}

function fromStorage(): CartItem[] {
  if (!browser) return [];
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as unknown;
    if (Array.isArray(parsed)) return parsed.filter(isCartItem); // legacy support
    if (typeof parsed === "object" && parsed !== null) {
      const p = parsed as Partial<CartPersistedV1>;
      if (p.version === SCHEMA_VERSION && Array.isArray(p.items)) {
        return p.items.filter(isCartItem);
      }
    }
    return [];
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
    const key = `${snapshot.id}:${snapshot.size ?? "-"}:${snapshot.color ?? "-"}`;
    const max = snapshot.maxQuantity ?? null;
    if (max !== null && max <= 0) return;
    const existing = this.items.find((i) => i.key === key);
    if (existing) {
      const next = existing.quantity + quantity;
      existing.quantity = max !== null ? Math.min(max, next) : next;
      return;
    }
    const initialQty = max !== null ? Math.min(max, quantity) : quantity;
    if (initialQty > 0) this.items.push({ key, quantity: initialQty, snapshot });
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
