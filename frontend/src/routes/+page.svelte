<script lang="ts">
  import { onMount } from "svelte";
  import ProductCard from "$lib/components/ProductCard.svelte";
  import { getProducts, listStoreCollections } from "$lib/api";
  import type { StoreCollection, UiProduct } from "$lib/types";

  let products = $state<UiProduct[]>([]);
  let storeCollections = $state<StoreCollection[]>([]);
  let visibleCount = $state(60);
  let loadingMore = $state(false);
  let loading = $state(true);
  let errorMessage = $state("");
  const endsAt = Date.now() + 1000 * 60 * 60 * 20;
  let now = $state(Date.now());
  const remainingSec = $derived(Math.max(0, Math.floor((endsAt - now) / 1000)));
  const hh = $derived(String(Math.floor(remainingSec / 3600)).padStart(2, "0"));
  const mm = $derived(String(Math.floor((remainingSec % 3600) / 60)).padStart(2, "0"));
  const ss = $derived(String(remainingSec % 60).padStart(2, "0"));
  const latestVisible = $derived(products.slice(0, visibleCount));
  let sentinel = $state<HTMLDivElement | null>(null);

  $effect(() => {
    const timer = setInterval(() => (now = Date.now()), 1000);
    getProducts()
      .then((p) => {
        products = p;
      })
      .catch(() => {
        errorMessage = "Failed to load products.";
      })
      .finally(() => {
        loading = false;
      });
    listStoreCollections().then((c) => {
      storeCollections = c.filter((x) => x.product_count > 0).slice(0, 6);
    });
    return () => clearInterval(timer);
  });

  onMount(() => {
    if (!sentinel) return;
    const obs = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (entry.isIntersecting && visibleCount < products.length) {
          loadingMore = true;
          setTimeout(() => {
            visibleCount = Math.min(visibleCount + 20, products.length);
            loadingMore = false;
          }, 280);
        }
      });
    }, { rootMargin: "600px 0px 600px 0px" });
    obs.observe(sentinel);
    return () => obs.disconnect();
  });
</script>

<svelte:head>
  <title>Home | Noir Drop</title>
  <meta name="description" content="Latest drops, trending products, and curated collections." />
</svelte:head>

<section class="-mx-4 mb-10 md:-mx-6 xl:-mx-8">
  <div class="relative aspect-[430/537] w-full overflow-hidden border-y border-zinc-300 bg-zinc-100 rounded-none md:h-[70vh] md:min-h-[420px] md:aspect-auto">
    <img
      src="https://picsum.photos/seed/home-hero-placeholder/2200/1100"
      alt="Hero placeholder"
      class="h-full w-full object-cover"
      loading="eager"
    />
    <div class="absolute inset-0 bg-black/20"></div>
    <div class="absolute inset-0 flex flex-col justify-end p-6 md:p-10">
      <p class="text-xs uppercase tracking-[0.24em] text-accent">Drop Live</p>
      <h1 class="mt-2 text-3xl font-black uppercase text-white md:text-6xl">Noir Season Collection</h1>
      <p class="mt-3 text-sm text-white">Ends in {hh}:{mm}:{ss}</p>
    </div>
  </div>
</section>

<section class="mb-10 lg:mb-16">
  <div class="mb-6 flex items-end justify-between px-4 md:px-0">
    <h2 class="text-sm font-bold uppercase tracking-widest text-zinc-900">
      {storeCollections.length > 0 ? "Collections" : "Curated Categories"}
    </h2>
    <a href="/shop" class="text-xs font-semibold uppercase tracking-wider text-zinc-500 hover:text-zinc-900 transition-colors">View all</a>
  </div>
  <div class="-mx-4 md:-mx-6 xl:-mx-8 px-1">
    {#if storeCollections.length > 0}
    <div class="grid grid-cols-2 gap-1 md:grid-cols-3 lg:grid-cols-6">
      {#each storeCollections as c (c.id)}
        <a href={`/collection/${encodeURIComponent(c.slug)}`} class="group relative aspect-[4/5] w-full overflow-hidden bg-zinc-100">
          {#if c.image_url}
            <img src={c.image_url} alt="" class="h-full w-full object-cover transition-transform duration-700 ease-out group-hover:scale-105" loading="lazy" />
          {:else}
            <img src={`https://picsum.photos/seed/col-${c.slug}/800/1000`} alt="" class="h-full w-full object-cover transition-transform duration-700 ease-out group-hover:scale-105" loading="lazy" />
          {/if}
          <div class="absolute inset-0 bg-black/20 transition-colors duration-500 group-hover:bg-black/40"></div>
          <div class="absolute inset-0 flex items-center justify-center px-2 text-center">
            <h3 class="text-sm md:text-base lg:text-xs xl:text-sm font-black uppercase tracking-widest text-white">{c.title}</h3>
          </div>
        </a>
      {/each}
    </div>
    {:else}
    <div class="grid grid-cols-2 gap-1 md:grid-cols-3 lg:grid-cols-6">
    <!-- Category 1 -->
    <a href="/shop" class="group relative aspect-[4/5] w-full overflow-hidden bg-zinc-100">
      <img src="https://picsum.photos/seed/cat-new/800/1000" alt="New Season" class="h-full w-full object-cover transition-transform duration-700 ease-out group-hover:scale-105" loading="lazy" />
      <div class="absolute inset-0 bg-black/20 transition-colors duration-500 group-hover:bg-black/40"></div>
      <div class="absolute inset-0 flex items-center justify-center">
        <h3 class="text-sm md:text-base lg:text-xs xl:text-sm font-black uppercase tracking-widest text-white">New Season</h3>
      </div>
    </a>
    
    <!-- Category 2 -->
    <a href="/shop" class="group relative aspect-[4/5] w-full overflow-hidden bg-zinc-100">
      <img src="https://picsum.photos/seed/cat-out/800/1000" alt="Outerwear" class="h-full w-full object-cover transition-transform duration-700 ease-out group-hover:scale-105" loading="lazy" />
      <div class="absolute inset-0 bg-black/20 transition-colors duration-500 group-hover:bg-black/40"></div>
      <div class="absolute inset-0 flex items-center justify-center">
        <h3 class="text-sm md:text-base lg:text-xs xl:text-sm font-black uppercase tracking-widest text-white">Outerwear</h3>
      </div>
    </a>

    <!-- Category 3 -->
    <a href="/shop" class="group relative aspect-[4/5] w-full overflow-hidden bg-zinc-100">
      <img src="https://picsum.photos/seed/cat-top/800/1000" alt="Tops" class="h-full w-full object-cover transition-transform duration-700 ease-out group-hover:scale-105" loading="lazy" />
      <div class="absolute inset-0 bg-black/20 transition-colors duration-500 group-hover:bg-black/40"></div>
      <div class="absolute inset-0 flex items-center justify-center">
        <h3 class="text-sm md:text-base lg:text-xs xl:text-sm font-black uppercase tracking-widest text-white">Tops</h3>
      </div>
    </a>

    <!-- Category 4 -->
    <a href="/shop" class="group relative aspect-[4/5] w-full overflow-hidden bg-zinc-100">
      <img src="https://picsum.photos/seed/cat-bot/800/1000" alt="Bottoms" class="h-full w-full object-cover transition-transform duration-700 ease-out group-hover:scale-105" loading="lazy" />
      <div class="absolute inset-0 bg-black/20 transition-colors duration-500 group-hover:bg-black/40"></div>
      <div class="absolute inset-0 flex items-center justify-center">
        <h3 class="text-sm md:text-base lg:text-xs xl:text-sm font-black uppercase tracking-widest text-white">Bottoms</h3>
      </div>
    </a>

    <!-- Category 5 -->
    <a href="/shop" class="group relative aspect-[4/5] w-full overflow-hidden bg-zinc-100">
      <img src="https://picsum.photos/seed/cat-acc/800/1000" alt="Accessories" class="h-full w-full object-cover transition-transform duration-700 ease-out group-hover:scale-105" loading="lazy" />
      <div class="absolute inset-0 bg-black/20 transition-colors duration-500 group-hover:bg-black/40"></div>
      <div class="absolute inset-0 flex items-center justify-center">
        <h3 class="text-sm md:text-base lg:text-xs xl:text-sm font-black uppercase tracking-widest text-white">Accessories</h3>
      </div>
    </a>

    <!-- Category 6 -->
    <a href="/shop" class="group relative aspect-[4/5] w-full overflow-hidden bg-zinc-100">
      <img src="https://picsum.photos/seed/cat-shoe/800/1000" alt="Footwear" class="h-full w-full object-cover transition-transform duration-700 ease-out group-hover:scale-105" loading="lazy" />
      <div class="absolute inset-0 bg-black/20 transition-colors duration-500 group-hover:bg-black/40"></div>
      <div class="absolute inset-0 flex items-center justify-center">
        <h3 class="text-sm md:text-base lg:text-xs xl:text-sm font-black uppercase tracking-widest text-white">Footwear</h3>
      </div>
    </a>
  </div>
    {/if}
  </div>
</section>





<section class="mb-10 -mx-4 md:-mx-6 xl:-mx-8">
  <h2 class="mb-3 px-4 text-sm font-bold uppercase tracking-wide md:px-6 xl:px-8">Shop Latest</h2>
  {#if loading}
    <div class="grid grid-cols-2 gap-[2px] lg:grid-cols-4 lg:gap-[3px]">
      {#each Array.from({ length: 12 }) as _}
        <div class="aspect-[2/3] animate-pulse bg-zinc-200"></div>
      {/each}
    </div>
  {:else}
    <div class="grid grid-cols-2 gap-[2px] lg:grid-cols-4 lg:gap-[3px]">
      {#each latestVisible as p (p.id)}
        <ProductCard product={p} />
      {/each}
    </div>
    <div class="flex items-center justify-center py-3 text-[10px] uppercase tracking-wide text-zinc-500">
      {#if visibleCount < products.length}
        {#if loadingMore}Loading more products...{:else}Scroll to load more{/if}
      {:else}
        You've viewed all products
      {/if}
    </div>
    <div bind:this={sentinel} class="h-8"></div>
  {/if}
</section>
