import { browser } from "$app/environment";
import type { UiProduct } from "$lib/types";

const STORAGE_KEY = "fashion_recent_views";
const SCHEMA_VERSION = 1;

type RecentPersistedV1 = {
  version: 1;
  items: UiProduct[];
};

function isUiProduct(input: unknown): input is UiProduct {
  if (typeof input !== "object" || input === null) return false;
  const r = input as Record<string, unknown>;
  return typeof r.id === "string" && typeof r.title === "string" && typeof r.slug === "string";
}

function fromStorage(): UiProduct[] {
  if (!browser) return [];
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as unknown;
    if (Array.isArray(parsed)) return parsed.filter(isUiProduct); // legacy support
    if (typeof parsed === "object" && parsed !== null) {
      const p = parsed as Partial<RecentPersistedV1>;
      if (p.version === SCHEMA_VERSION && Array.isArray(p.items)) {
        return p.items.filter(isUiProduct);
      }
    }
    return [];
  } catch {
    return [];
  }
}

class RecentViewsState {
  items = $state<UiProduct[]>(fromStorage());

  constructor() {
    $effect.root(() => {
      $effect(() => {
        if (!browser) return;
        const payload: RecentPersistedV1 = {
          version: SCHEMA_VERSION,
          items: this.items.slice(0, 10)
        };
        localStorage.setItem(STORAGE_KEY, JSON.stringify(payload));
      });
    });
  }

  push(product: UiProduct) {
    this.items = [product, ...this.items.filter((x) => x.id !== product.id)].slice(0, 10);
  }
}

export const recentViews = new RecentViewsState();
