<script lang="ts">
  import { goto } from "$app/navigation";
  import { page } from "$app/state";
  import ProductCard from "$lib/components/ProductCard.svelte";
  import { getProducts } from "$lib/api";
  import type { UiProduct } from "$lib/types";

  let all = $state<UiProduct[]>([]);
  let visibleCount = $state(12);
  let loading = $state(true);
  let errorMessage = $state("");

  const size = $derived(page.url.searchParams.get("size") ?? "");
  const color = $derived(page.url.searchParams.get("color") ?? "");
  const category = $derived(page.url.searchParams.get("category") ?? "");
  const minPrice = $derived(Number(page.url.searchParams.get("min") ?? "0"));
  const maxPrice = $derived(Number(page.url.searchParams.get("max") ?? "9999"));

  $effect(() => {
    getProducts()
      .then((res) => {
        all = res;
      })
      .catch(() => {
        errorMessage = "Unable to load shop products.";
      })
      .finally(() => {
        loading = false;
      });
  });

  const filtered = $derived(
    all.filter((p) => {
      if (size && !p.sizeOptions.includes(size)) return false;
      if (color && p.color.toLowerCase() !== color.toLowerCase()) return false;
      if (category && p.category !== category) return false;
      if (p.price < minPrice || p.price > maxPrice) return false;
      return true;
    })
  );

  const shown = $derived(filtered.slice(0, visibleCount));

  function setParam(key: string, val: string) {
    const sp = new URLSearchParams(page.url.searchParams);
    if (!val) sp.delete(key);
    else sp.set(key, val);
    goto(`/shop?${sp.toString()}`, { keepFocus: true, noScroll: true });
  }
</script>

<svelte:head>
  <title>Shop | JS FASHION</title>
  <meta name="description" content="Shop all products with size, price, color and category filters." />
</svelte:head>

<div class="mb-6 grid gap-3 rounded-xl border border-zinc-300 p-4 md:grid-cols-5">
  <select class="rounded border border-zinc-300 bg-white px-2 py-2 text-xs" value={size} onchange={(e) => setParam("size", (e.currentTarget as HTMLSelectElement).value)}>
    <option value="">All sizes</option><option>S</option><option>M</option><option>L</option><option>XL</option>
  </select>
  <select class="rounded border border-zinc-300 bg-white px-2 py-2 text-xs" value={color} onchange={(e) => setParam("color", (e.currentTarget as HTMLSelectElement).value)}>
    <option value="">All colors</option><option>Black</option><option>White</option><option>Pink</option><option>Olive</option>
  </select>
  <select class="rounded border border-zinc-300 bg-white px-2 py-2 text-xs" value={category} onchange={(e) => setParam("category", (e.currentTarget as HTMLSelectElement).value)}>
    <option value="">All categories</option><option value="streetwear">Streetwear</option><option value="oversized">Oversized</option><option value="minimal">Minimal</option><option value="accessories">Accessories</option>
  </select>
  <input class="rounded border border-zinc-300 bg-white px-2 py-2 text-xs" type="number" placeholder="Min price" value={minPrice} onchange={(e) => setParam("min", (e.currentTarget as HTMLInputElement).value)} />
  <input class="rounded border border-zinc-300 bg-white px-2 py-2 text-xs" type="number" placeholder="Max price" value={maxPrice} onchange={(e) => setParam("max", (e.currentTarget as HTMLInputElement).value)} />
</div>

{#if loading}
  <div class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
    {#each Array.from({ length: 8 }) as _}
      <div class="h-72 animate-pulse rounded-xl bg-zinc-200"></div>
    {/each}
  </div>
{:else if errorMessage}
  <p class="text-sm text-rose-400">{errorMessage}</p>
{:else}
  <div class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
    {#each shown as p (p.id)}
      <ProductCard product={p} />
    {/each}
  </div>
{/if}

{#if shown.length < filtered.length}
  <div class="mt-8 text-center">
    <button class="rounded border border-zinc-300 px-4 py-2 text-xs uppercase" onclick={() => (visibleCount += 8)}>Load more</button>
  </div>
{/if}
