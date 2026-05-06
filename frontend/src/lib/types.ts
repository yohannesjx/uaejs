export type ProductColorSwatch = {
  name: string;
  imageUrl: string | null;
  images: string[];
};

/** Public storefront collection metadata (manual merchandising groups). */
export type StoreCollection = {
  id: string;
  title: string;
  slug: string;
  description?: string | null;
  image_url?: string | null;
  product_count: number;
};

export type UiProduct = {
  id: string;
  slug: string;
  title: string;
  price: number;
  compareAtPrice: number | null;
  imageUrl: string | null;
  images: string[];
  category: string;
  color: string;
  colors: string[];
  colorSwatches: ProductColorSwatch[];
  sizeOptions: string[];
  tags: string[];
  description: string;
  inventory: number | null;
  variants: ProductVariantAvailability[];
};

export type ProductVariantAvailability = {
  size: string;
  color: string;
  stock: number;
  isAvailable: boolean;
};

export type CartItemSnapshot = {
  id: string;
  slug: string;
  title: string;
  price: number;
  imageUrl: string | null;
  size?: string;
  color?: string;
  maxQuantity?: number | null;
};

export type CartItem = {
  key: string;
  quantity: number;
  snapshot: CartItemSnapshot;
};
