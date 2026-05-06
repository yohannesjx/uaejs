<script lang="ts">
  import { Heart, ShoppingBag } from "lucide-svelte";
  import type { UiProduct } from "$lib/types";
  import ResponsiveImage from "./ResponsiveImage.svelte";

  let { product }: { product: UiProduct } = $props();

  const colorMap: Record<string, string> = {
    black: "#111111",
    white: "#ffffff",
    charcoal: "#3f3f46",
    gray: "#6b7280",
    grey: "#6b7280",
    pink: "#f9a8d4",
    olive: "#4B5320",
    sand: "#C2B280",
    beige: "#D6C4A1",
    red: "#dc2626",
    blue: "#2563eb",
    green: "#16a34a",
    brown: "#8b5a2b"
  };

  function colorToCss(colorName: string): string {
    const key = colorName.trim().toLowerCase();
    return colorMap[key] ?? key;
  }

  const shownSwatches = $derived((product.colorSwatches ?? []).slice(0, 4));
  const extraSwatches = $derived(Math.max(0, (product.colorSwatches?.length ?? 0) - shownSwatches.length));

  const discountPercent = $derived(
    product.compareAtPrice && product.compareAtPrice > product.price
      ? Math.round(((product.compareAtPrice - product.price) / product.compareAtPrice) * 100)
      : 45
  );
</script>

<article class="product-card bg-white">
  <!-- Image block — full bleed, no padding -->
  <a href={`/product/${product.slug}`} class="block relative">
    <div class="relative overflow-hidden aspect-[2/3] bg-zinc-100">
      <ResponsiveImage src={product.imageUrl} alt={product.title} aspect="aspect-[2/3]" />
    </div>
    <!-- Circular quick-add cart button -->
    <button
      type="button"
      class="quick-add-btn"
      aria-label="Quick add to cart"
    >
      <ShoppingBag size={15} strokeWidth={1.8} />
    </button>
  </a>

  <!-- Info -->
  <div class="px-[6px] pt-[7px] pb-[10px] space-y-[3px]">
    <!-- Title + Heart -->
    <div class="flex items-start justify-between gap-1">
      <h3 class="text-[11px] leading-[1.35] font-medium text-zinc-900 line-clamp-2 flex-1 min-w-0">
        {product.title}
      </h3>
      <button
        type="button"
        class="shrink-0 mt-[1px] text-zinc-400 hover:text-rose-500 transition-colors"
        aria-label="Wishlist"
      >
        <Heart size={14} strokeWidth={1.8} />
      </button>
    </div>

    <!-- Prices -->
    <div class="flex items-baseline gap-1.5 flex-wrap">
      <span class="text-[12.5px] font-extrabold text-[#c8102e] leading-none tracking-tight">
        {product.price.toFixed(2)} AED
      </span>
      {#if product.compareAtPrice}
        <span class="text-[11px] text-zinc-400 line-through leading-none">
          {product.compareAtPrice.toFixed(2)} AED
        </span>
      {/if}
    </div>



    <!-- Color swatches -->
    {#if shownSwatches.length > 0}
    <div class="flex items-center gap-[5px] pt-[2px]">
      {#each shownSwatches as swatch}
        {#if swatch.imageUrl}
          <span class="swatch swatch-image" title={swatch.name}>
            <img src={swatch.imageUrl} alt={swatch.name} />
          </span>
        {:else}
          <span class="swatch" style={`background:${colorToCss(swatch.name)};`} title={swatch.name}></span>
        {/if}
      {/each}
      {#if extraSwatches > 0}
        <span class="text-[10.5px] font-bold text-zinc-500 leading-none">+{extraSwatches}</span>
      {/if}
    </div>
    {/if}
  </div>
</article>

<style>
  .product-card {
    position: relative;
  }

  /* Circular quick-add — glassy, bottom-right of image */
  .quick-add-btn {
    position: absolute;
    bottom: 9px;
    right: 9px;
    width: 34px;
    height: 34px;
    border-radius: 50%;
    background: rgba(255, 255, 255, 0.9);
    border: 1px solid rgba(0, 0, 0, 0.1);
    display: flex;
    align-items: center;
    justify-content: center;
    cursor: pointer;
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
    box-shadow:
      0 2px 8px rgba(0, 0, 0, 0.16),
      0 0 0 0.5px rgba(0, 0, 0, 0.04);
    transition:
      background 0.15s ease,
      transform 0.15s ease,
      box-shadow 0.15s ease;
    color: #111;
    z-index: 2;
  }
  .quick-add-btn:hover {
    background: #fff;
    transform: scale(1.1);
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.22);
  }
  .quick-add-btn:active {
    transform: scale(0.95);
  }

  /* Premium round swatches */
  .swatch {
    display: inline-block;
    width: 17px;
    height: 17px;
    border-radius: 50%;
    border: 1.5px solid rgba(0, 0, 0, 0.15);
    box-shadow:
      inset 0 0 0 1.5px rgba(255, 255, 255, 0.25),
      0 1px 4px rgba(0, 0, 0, 0.1);
    flex-shrink: 0;
    cursor: pointer;
    transition: transform 0.12s ease, box-shadow 0.12s ease;
  }
  .swatch-image {
    overflow: hidden;
    background: #f4f4f5;
  }
  .swatch-image img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
  .swatch:hover {
    transform: scale(1.25);
    box-shadow:
      inset 0 0 0 1.5px rgba(255, 255, 255, 0.35),
      0 2px 8px rgba(0, 0, 0, 0.2);
  }
</style>
