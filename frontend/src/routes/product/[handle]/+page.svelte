<script lang="ts">
  import { page } from "$app/state";
  import { fade } from "svelte/transition";
  import { cart } from "$lib/state/cart.svelte";
  import { recentViews } from "$lib/state/recentViews.svelte";
  import { getProduct, getRelatedProducts, invalidateProductCache } from "$lib/api";
  import type { UiProduct } from "$lib/types";
  import ProductCard from "$lib/components/ProductCard.svelte";
  import Button from "$lib/components/Button.svelte";
  import Modal from "$lib/components/Modal.svelte";
  import ResponsiveImage from "$lib/components/ResponsiveImage.svelte";

  let product = $state<UiProduct | null>(null);
  let related = $state<UiProduct[]>([]);
  let selectedSize = $state("");
  let selectedColor = $state("");

  let sizeModalOpen = $state(false);
  let lightboxOpen = $state(false);
  let lightboxIndex = $state(0);
  let touchStartX = $state<number | null>(null);
  let touchStartY = $state<number | null>(null);

  /** Swipe-back (history): left-edge or details panel, disabled while lightbox open */
  type SwipeBackMode = "edge" | "details";
  let swipeBackStart = $state<{ x: number; y: number; mode: SwipeBackMode } | null>(null);
  const SWIPE_BACK_EDGE_X = 36;
  const SWIPE_BACK_EDGE_MIN_DX = 64;
  const SWIPE_BACK_DETAILS_MIN_DX = 88;
  let stockNotice = $state("");
  let loading = $state(true);
  let errorMessage = $state("");
  let relatedLoadedFor = $state("");

  /** Normalize variant dimension strings so "M " matches "M" and empty size rows still aggregate. */
  function dim(s: string | undefined | null) {
    return (s ?? "").trim();
  }

  /** Same image URL twice (e.g. thumbnail + hero) should count as one PDP photo. */
  function dedupeImageUrls(urls: string[]): string[] {
    const seen = new Set<string>();
    const out: string[] = [];
    for (const u of urls) {
      const t = (u ?? "").trim();
      if (!t || seen.has(t)) continue;
      seen.add(t);
      out.push(t);
    }
    return out;
  }

  $effect(() => {
    const handle = page.params.handle ?? "";
    const syncProduct = () => getProduct(handle)
      .then((p) => {
        product = p;
        if (!p) {
          errorMessage = "Product not found.";
          return;
        }
        const sizeKeys = p.sizeOptions.map((x) => dim(x));
        if (!selectedSize || !sizeKeys.includes(dim(selectedSize))) selectedSize = p.sizeOptions[0] ?? "";
        const colorKeys = p.colors.map((c) => dim(c));
        if (!selectedColor || !colorKeys.includes(dim(selectedColor))) selectedColor = dim(p.color);
        if (relatedLoadedFor !== p.id) {
          recentViews.push(p);
          relatedLoadedFor = p.id;
          getRelatedProducts(p.id).then((r) => (related = r));
        }
      })
      .catch(() => {
        errorMessage = "Unable to load product.";
      })
      .finally(() => {
        loading = false;
      });
    syncProduct();
    const timer = setInterval(() => {
      if (!handle) return;
      invalidateProductCache(handle);
      void syncProduct();
    }, 8000);
    return () => clearInterval(timer);
  });

  const urgency = $derived(product && product.inventory !== null && product.inventory <= 5 ? `Only ${product.inventory} left` : "");
  const pageTitle = $derived(product ? `${product.title} | Noir Drop` : "Product | Noir Drop");
  const pageDescription = $derived(product?.description || "Product details and related looks.");
  const availableColorSwatches = $derived(product ? product.colorSwatches : []);

  /** Show sizes when any variant has a real size (e.g. single "M"). Hide only the default "[\"\"]" → "One size" placeholder when no size was set on variants. */
  const hasSizePicker = $derived.by(() => {
    if (!product) return false;
    return product.sizeOptions.some((s) => dim(s) !== "");
  });

  const hasColorPicker = $derived.by(() => {
    if (!product) return false;
    const set = new Set<string>();
    for (const sw of product.colorSwatches) {
      const n = dim(sw.name);
      if (!n || n.toLowerCase() === "unknown") continue;
      set.add(n);
    }
    return set.size > 1;
  });
  const selectedVariantStock = $derived.by(() => {
    if (!product) return 0;
    const selSz = dim(selectedSize);
    const selCol = dim(selectedColor);
    const anySize = product.variants.some((v) => dim(v.size) !== "");
    const anyColor = product.variants.some((v) => dim(v.color) !== "");
    const sizeOk = (vs: string) => !anySize || dim(vs) === selSz;
    const colorOk = (vc: string) =>
      !anyColor || selCol.toLowerCase() === "unknown" || dim(vc) === selCol;
    const exact = product.variants.find((v) => sizeOk(v.size) && colorOk(v.color));
    if (exact) return exact.stock;
    return product.inventory ?? 0;
  });
  const canAddToCart = $derived(selectedVariantStock > 0);
  const sizeStockByOption = $derived.by(() => {
    const out = new Map<string, number>();
    if (!product) return out;
    const selCol = dim(selectedColor);
    const anyColor = product.variants.some((v) => dim(v.color) !== "");
    for (const size of product.sizeOptions) {
      const key = dim(size);
      const stock = product.variants
        .filter(
          (v) =>
            dim(v.size) === key &&
            (!anyColor || selCol.toLowerCase() === "unknown" || dim(v.color) === selCol)
        )
        .reduce((sum, v) => sum + v.stock, 0);
      out.set(size, stock);
    }
    return out;
  });
  const colorStockByOption = $derived.by(() => {
    const out = new Map<string, number>();
    if (!product) return out;
    const selSz = dim(selectedSize);
    const anySize = product.variants.some((v) => dim(v.size) !== "");
    for (const swatch of availableColorSwatches) {
      const key = dim(swatch.name);
      const stock = product.variants
        .filter((v) => dim(v.color) === key && (!anySize || dim(v.size) === selSz))
        .reduce((sum, v) => sum + v.stock, 0);
      out.set(key, stock);
    }
    return out;
  });

  const displayedImages = $derived(
    product && dim(selectedColor)
      ? (product.colorSwatches.find((s) => dim(s.name) === dim(selectedColor))?.images?.length
          ? product.colorSwatches.find((s) => dim(s.name) === dim(selectedColor))!.images
          : product.images)
      : (product?.images ?? [])
  );

  /** Unique gallery URLs; falls back to product thumbnail when the images array is empty. */
  const galleryImages = $derived.by(() => {
    const list = dedupeImageUrls(displayedImages);
    if (list.length > 0) return list;
    const fallback = product?.imageUrl?.trim() ?? "";
    return fallback ? [fallback] : [];
  });

  function colorToCss(colorName: string): string {
    const key = colorName.trim().toLowerCase();
    const map: Record<string, string> = {
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
    return map[key] ?? key;
  }

  function addCurrentToCart() {
    if (!product) return;
    if (selectedVariantStock <= 0) {
      stockNotice = "This variant is out of stock.";
      return;
    }
    stockNotice = "";
    cart.add({
      id: product.id,
      slug: product.slug,
      title: product.title,
      price: product.price,
      imageUrl: product.imageUrl,
      size: selectedSize,
      color: selectedColor || undefined,
      maxQuantity: selectedVariantStock
    });
  }

  function openLightbox(index: number) {
    lightboxIndex = index;
    lightboxOpen = true;
  }

  function closeLightbox() {
    lightboxOpen = false;
  }

  function nextImage() {
    if (!galleryImages.length) return;
    lightboxIndex = (lightboxIndex + 1) % galleryImages.length;
  }

  function prevImage() {
    if (!galleryImages.length) return;
    lightboxIndex = (lightboxIndex - 1 + galleryImages.length) % galleryImages.length;
  }

  function onLightboxKeydown(event: KeyboardEvent) {
    if (!lightboxOpen) return;
    if (event.key === "ArrowRight") nextImage();
    if (event.key === "ArrowLeft") prevImage();
    if (event.key === "Escape") closeLightbox();
  }

  function onTouchStart(event: TouchEvent) {
    touchStartX = event.touches[0]?.clientX ?? null;
    touchStartY = event.touches[0]?.clientY ?? null;
  }

  function onTouchEnd(event: TouchEvent) {
    if (touchStartX === null || touchStartY === null) return;
    const endX = event.changedTouches[0]?.clientX ?? touchStartX;
    const endY = event.changedTouches[0]?.clientY ?? touchStartY;
    const delta = endX - touchStartX;
    const deltaY = endY - touchStartY;
    const absX = Math.abs(delta);
    const absY = Math.abs(deltaY);

    // Vertical swipe (up/down) closes the lightbox on mobile.
    if (absY > 60 && absY > absX) {
      closeLightbox();
    } else if (absX > 40) {
      if (delta < 0) nextImage();
      else prevImage();
    }
    touchStartX = null;
    touchStartY = null;
  }

  function onSwipeBackTouchStart(e: TouchEvent) {
    if (lightboxOpen) return;
    const t = e.touches[0];
    if (!t) return;
    const x = t.clientX;
    const y = t.clientY;
    const target = e.target;
    let inDetails = false;
    if (target instanceof Element) {
      inDetails = !!target.closest("[data-pdp-swipe-back-details]");
    }
    if (x <= SWIPE_BACK_EDGE_X) {
      swipeBackStart = { x, y, mode: "edge" };
    } else if (inDetails) {
      swipeBackStart = { x, y, mode: "details" };
    } else {
      swipeBackStart = null;
    }
  }

  function onSwipeBackTouchEnd(e: TouchEvent) {
    const start = swipeBackStart;
    swipeBackStart = null;
    if (lightboxOpen || !start) return;
    const t = e.changedTouches[0];
    if (!t) return;
    const dx = t.clientX - start.x;
    const dy = t.clientY - start.y;
    const minDx = start.mode === "edge" ? SWIPE_BACK_EDGE_MIN_DX : SWIPE_BACK_DETAILS_MIN_DX;
    if (dx > minDx && dx > Math.abs(dy) * 1.15) {
      history.back();
    }
  }
</script>

<svelte:head>
  <title>{pageTitle}</title>
  <meta name="description" content={pageDescription} />
  <meta property="og:title" content={pageTitle} />
  <meta property="og:description" content={pageDescription} />
</svelte:head>

<svelte:window onkeydown={onLightboxKeydown} />

{#if loading}
  <div class="grid gap-8 lg:grid-cols-[1fr_400px] xl:grid-cols-[1fr_450px]">
    <div class="h-[630px] w-[378px] max-w-[88vw] animate-pulse bg-zinc-200"></div>
    <div class="space-y-4 px-4 lg:px-0">
      <div class="h-8 w-2/3 animate-pulse bg-zinc-200"></div>
      <div class="h-4 w-1/3 animate-pulse bg-zinc-200"></div>
      <div class="h-12 w-full animate-pulse bg-zinc-200 mt-8"></div>
    </div>
  </div>
{:else if errorMessage}
  <div class="px-4 py-20 text-center">
    <p class="text-sm font-bold uppercase tracking-widest text-rose-500">{errorMessage}</p>
    <a href="/shop" class="mt-4 inline-block text-xs font-semibold uppercase tracking-widest text-zinc-500 hover:text-black">Return to Shop</a>
  </div>
{:else if product}
  <!-- Gesture layer: left-edge / details swipe-right → history.back(); no focus trap -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="relative min-w-0"
    ontouchstart={onSwipeBackTouchStart}
    ontouchend={onSwipeBackTouchEnd}
  >
  <div class="-mx-4 md:-mx-6 xl:-mx-8 lg:mx-0 lg:grid lg:grid-cols-[1fr_400px] xl:grid-cols-[1fr_450px] lg:gap-12 lg:items-start">
    
    <!-- Image Gallery: one unique URL → full width; multiple → horizontal snap strip (unchanged). -->
    {#if galleryImages.length > 1}
      <div class="flex overflow-x-auto snap-x snap-mandatory hide-scrollbar gap-[2px] lg:flex-col lg:overflow-visible lg:gap-4">
        {#each galleryImages as img, idx}
          <div class="h-[630px] w-[378px] max-w-[88vw] shrink-0 snap-center overflow-hidden flex-none">
            <button type="button" class="h-full w-full" onclick={() => openLightbox(idx)} aria-label={`Open image ${idx + 1}`}>
              <img src={img} class="h-full w-full object-cover object-center" alt={product.title} />
            </button>
          </div>
        {/each}
      </div>
    {:else}
      <div class="w-full min-w-0 max-w-[88vw] mx-auto lg:max-w-none lg:mx-0">
        <div class="w-full overflow-hidden">
          <button type="button" class="block w-full touch-manipulation" onclick={() => openLightbox(0)} aria-label="Open product image">
            <img
              src={galleryImages[0] ?? product.imageUrl ?? ""}
              class="w-full aspect-[3/4] object-cover object-center lg:aspect-auto lg:h-[630px] lg:w-full lg:max-w-full"
              alt={product.title}
            />
          </button>
        </div>
      </div>
    {/if}

    <!-- Product Details (Sticky on Desktop); swipe-right-back on mobile via data attr -->
    <div
      class="px-4 md:px-6 xl:px-8 lg:px-0 pt-8 pb-32 lg:pb-8 lg:sticky lg:top-[100px]"
      data-pdp-swipe-back-details
    >
      <div class="mb-8">
        <h1 class="text-sm md:text-base font-normal uppercase tracking-widest text-zinc-600">{product.title}</h1>
        <p class="text-2xl md:text-3xl font-black text-black mt-1">{product.price.toFixed(2)} AED</p>
      </div>

      <div class="space-y-6">
        <!-- Color (only when multiple real colors — hide single default swatch) -->
        {#if hasColorPicker}
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <span class="text-xs font-bold uppercase tracking-widest">Color <span class="text-zinc-500 font-normal ml-2">{selectedColor}</span></span>
          </div>
          <div class="flex items-center gap-3">
            {#each availableColorSwatches as swatch}
              {@const swatchStock = colorStockByOption.get(dim(swatch.name)) ?? 0}
              {@const isSwatchDisabled = swatchStock <= 0}
              <button 
                class="relative flex h-8 w-8 items-center justify-center overflow-hidden rounded-full disabled:cursor-not-allowed disabled:opacity-45 disabled:grayscale disabled:brightness-90 {dim(selectedColor) === dim(swatch.name) ? 'ring-1 ring-black ring-offset-2' : 'hover:ring-1 hover:ring-zinc-300 hover:ring-offset-2'} transition-all"
                onclick={() => { selectedColor = swatch.name; stockNotice = ""; }}
                aria-label={swatch.name}
                disabled={isSwatchDisabled}
              >
                {#if swatch.imageUrl}
                  <img src={swatch.imageUrl} alt={swatch.name} class="h-6 w-6 rounded-full object-cover" />
                {:else}
                  <span class="h-6 w-6 rounded-full" style={`background:${colorToCss(swatch.name)};`}></span>
                {/if}
              </button>
            {/each}
          </div>
        </div>
        {/if}

        <!-- Size (only when multiple distinct sizes — hide "One size" / default-only) -->
        {#if hasSizePicker}
        <div class="space-y-3">
          <div class="flex items-center justify-between">
            <span class="text-xs font-bold uppercase tracking-widest">Size</span>
          </div>
          <div class="grid grid-cols-4 gap-2">
            {#each product.sizeOptions as s}
              {@const sizeStock = sizeStockByOption.get(s) ?? 0}
              {@const isSizeDisabled = sizeStock <= 0}
              <button
                class="relative border py-3 text-xs font-semibold uppercase tracking-widest transition-colors disabled:cursor-not-allowed disabled:opacity-40 {dim(selectedSize) === dim(s) ? 'border-black bg-black text-white' : 'border-zinc-300 text-black hover:border-black'}"
                onclick={() => { selectedSize = s; stockNotice = ""; }}
                disabled={isSizeDisabled}
              >
                {s === "" ? "One size" : s}
              </button>
            {/each}
          </div>
        </div>
        {/if}

        <!-- Desktop Add to Cart -->
        <div class="pt-6 hidden lg:block">
          <button
            class="w-full bg-black text-white py-4 text-xs font-black uppercase tracking-[0.2em] transition-colors disabled:cursor-not-allowed disabled:bg-zinc-400"
            onclick={addCurrentToCart}
            disabled={!canAddToCart}
          >
            {#if canAddToCart}
              Add to Cart - {product.price.toFixed(2)} AED
            {:else}
              Out of Stock
            {/if}
          </button>
          {#if !canAddToCart}
            <p class="mt-2 text-[11px] uppercase tracking-wider text-rose-500">Selected variant is out of stock</p>
          {:else if stockNotice}
            <p class="mt-2 text-[11px] uppercase tracking-wider text-rose-500">{stockNotice}</p>
          {/if}
        </div>

        <!-- Accordions -->
        <div class="pt-8">
          <div class="border-t border-zinc-200 py-4">
            <details class="group cursor-pointer">
              <summary class="flex items-center justify-between font-bold text-xs uppercase tracking-widest list-none">
                Shipping & Returns
                <span class="group-open:rotate-180 transition-transform text-lg font-normal leading-none">+</span>
              </summary>
              <div class="mt-4 text-xs text-zinc-500 leading-relaxed space-y-2">
                <p>Free standard shipping on all orders over $200. Express shipping available at checkout.</p>
                <p>Returns accepted within 14 days of delivery. Items must be in original unworn condition with all tags attached.</p>
              </div>
            </details>
          </div>
          <div class="border-t border-zinc-200 py-4">
            <details class="group cursor-pointer" open>
              <summary class="flex items-center justify-between font-bold text-xs uppercase tracking-widest list-none">
                Product Details
                <span class="group-open:rotate-180 transition-transform text-lg font-normal leading-none">+</span>
              </summary>
              <div class="mt-4 text-xs text-zinc-500 leading-relaxed space-y-2">
                <p>{product.description}</p>
                <ul class="list-inside list-disc mt-2 space-y-1">
                  <li>Premium heavy-weight fabric</li>
                  <li>Oversized fit</li>
                  <li>Machine wash cold, hang dry</li>
                </ul>
              </div>
            </details>
          </div>
          <div class="border-t border-zinc-200"></div>
        </div>
      </div>
    </div>
  </div>

  {#if lightboxOpen}
    <div
      class="fixed inset-0 z-[120] bg-black/95"
      role="dialog"
      aria-modal="true"
      ontouchstart={onTouchStart}
      ontouchend={onTouchEnd}
    >
      <button class="absolute right-4 top-4 z-20 text-white/90 text-3xl leading-none" onclick={closeLightbox} aria-label="Close gallery">
        &times;
      </button>

      {#if galleryImages.length > 1}
        <button class="absolute left-4 top-1/2 z-20 -translate-y-1/2 text-white/90 text-4xl leading-none hidden md:block" onclick={prevImage} aria-label="Previous image">
          &#10094;
        </button>
        <button class="absolute right-4 top-1/2 z-20 -translate-y-1/2 text-white/90 text-4xl leading-none hidden md:block" onclick={nextImage} aria-label="Next image">
          &#10095;
        </button>
      {/if}

      <div class="flex h-full w-full items-center justify-center px-4">
        {#key `${lightboxIndex}-${galleryImages[lightboxIndex]}`}
          <img
            src={galleryImages[lightboxIndex] ?? product.imageUrl ?? ""}
            alt={`${product.title} image ${lightboxIndex + 1}`}
            class="max-h-[92vh] max-w-[96vw] object-contain"
            transition:fade={{ duration: 180 }}
          />
        {/key}
      </div>

      {#if galleryImages.length > 1}
        <div class="absolute bottom-6 left-1/2 -translate-x-1/2 text-white/80 text-xs tracking-widest">
          {lightboxIndex + 1} / {galleryImages.length}
        </div>
      {/if}
    </div>
  {/if}

  <!-- You May Also Like Section -->
  <section class="mt-16 lg:mt-32 mb-10 -mx-4 md:-mx-6 xl:-mx-8 px-1">
    <div class="mb-6 px-4 md:px-0 text-center lg:text-left lg:px-4">
      <h2 class="text-sm font-bold uppercase tracking-widest">You May Also Like</h2>
    </div>
    <div class="grid grid-cols-2 gap-1 md:grid-cols-3 lg:grid-cols-6">
      {#each related.slice(0, 6) as p (p.id)}
        <div class="w-full">
          <ProductCard product={p} />
        </div>
      {/each}
    </div>
  </section>

  <!-- Mobile Sticky Add to Cart -->
  <div class="fixed bottom-0 left-0 right-0 z-50 p-4 bg-white/90 backdrop-blur-md border-t border-zinc-200 lg:hidden shadow-[0_-4px_20px_rgba(0,0,0,0.05)]">
    <button
      class="w-full bg-black text-white py-4 text-xs font-black uppercase tracking-[0.2em] transition-colors disabled:cursor-not-allowed disabled:bg-zinc-400"
      onclick={addCurrentToCart}
      disabled={!canAddToCart}
    >
      {#if canAddToCart}
        Add to Cart - {product.price.toFixed(2)} AED
      {:else}
        Out of Stock
      {/if}
    </button>
    {#if !canAddToCart}
      <p class="mt-2 text-center text-[11px] uppercase tracking-wider text-rose-500">Selected variant is out of stock</p>
    {:else if stockNotice}
      <p class="mt-2 text-center text-[11px] uppercase tracking-wider text-rose-500">{stockNotice}</p>
    {/if}
  </div>
  </div>
{/if}

<style>
  /* Hide scrollbar for gallery */
  .hide-scrollbar::-webkit-scrollbar {
    display: none;
  }
  .hide-scrollbar {
    -ms-overflow-style: none;
    scrollbar-width: none;
  }
  
  /* Details summary marker hide for Safari */
  details > summary::-webkit-details-marker {
    display: none;
  }
</style>
