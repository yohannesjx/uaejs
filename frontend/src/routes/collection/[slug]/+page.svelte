<script lang="ts">
  import { page } from "$app/state";
  import ProductCard from "$lib/components/ProductCard.svelte";
  import { getProducts, listStoreCollections } from "$lib/api";
  import type { StoreCollection, UiProduct } from "$lib/types";

  let slug = $derived(page.params.slug ?? "");
  let products = $state<UiProduct[]>([]);
  let collectionMeta = $state<StoreCollection | null>(null);
  let loading = $state(true);
  let notFound = $state(false);
  let errorMessage = $state("");

  $effect(() => {
    const s = slug;
    if (!s) return;
    loading = true;
    notFound = false;
    errorMessage = "";
    void listStoreCollections().then((list) => {
      collectionMeta = list.find((c) => c.slug === s) ?? null;
    });
    getProducts(s)
      .then((p) => {
        products = p;
      })
      .catch((e: unknown) => {
        const st = typeof e === "object" && e !== null && "status" in e ? (e as { status?: number }).status : undefined;
        if (st === 404) {
          notFound = true;
          products = [];
        } else {
          errorMessage = "Unable to load this collection.";
          products = [];
        }
      })
      .finally(() => {
        loading = false;
      });
  });
</script>

<svelte:head>
  <title>{notFound ? "Collection" : collectionMeta?.title || slug} | Noir Drop</title>
  <meta name="description" content={collectionMeta?.description || "Shop products in this collection."} />
</svelte:head>

<div class="mb-8 px-1">
  {#if loading}
    <div class="h-8 max-w-xs animate-pulse rounded bg-zinc-200"></div>
  {:else if notFound}
    <h1 class="text-lg font-black uppercase tracking-widest text-zinc-900">Collection not found</h1>
    <p class="mt-2 text-sm text-zinc-600">This link may be outdated or the collection was removed.</p>
    <a href="/shop" class="mt-6 inline-block text-xs font-semibold uppercase tracking-wider text-zinc-500 hover:text-zinc-900">Back to shop</a>
  {:else if errorMessage}
    <p class="text-sm text-rose-500">{errorMessage}</p>
  {:else}
    {#if collectionMeta?.image_url}
      <div class="mb-6 aspect-[21/9] w-full max-w-3xl overflow-hidden rounded-lg border border-zinc-200 bg-zinc-100">
        <img src={collectionMeta.image_url} alt="" class="h-full w-full object-cover" loading="eager" />
      </div>
    {/if}
    <p class="text-xs uppercase tracking-[0.24em] text-accent">Collection</p>
    <h1 class="mt-2 text-2xl font-black uppercase tracking-wide text-zinc-900 md:text-3xl">
      {collectionMeta?.title || slug.replace(/-/g, " ")}
    </h1>
    {#if collectionMeta?.description}
      <p class="mt-2 max-w-2xl text-sm text-zinc-600">{collectionMeta.description}</p>
    {/if}
    <p class="mt-2 text-sm text-zinc-600">{products.length} {products.length === 1 ? "product" : "products"}</p>
  {/if}
</div>

{#if !notFound && !errorMessage}
  {#if loading}
    <div class="grid grid-cols-2 gap-[2px] lg:grid-cols-4 lg:gap-[3px]">
      {#each Array.from({ length: 8 }) as _}
        <div class="aspect-[2/3] animate-pulse bg-zinc-200"></div>
      {/each}
    </div>
  {:else if products.length === 0}
    <p class="text-sm text-zinc-500">No products in this collection yet.</p>
  {:else}
    <div class="grid grid-cols-2 gap-[2px] lg:grid-cols-4 lg:gap-[3px]">
      {#each products as p (p.id)}
        <ProductCard product={p} />
      {/each}
    </div>
  {/if}
{/if}
