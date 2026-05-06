<script lang="ts">
  import { fly } from "svelte/transition";
  import { cart } from "$lib/state/cart.svelte";
  import Button from "./Button.svelte";
  let { open = false, onClose = () => {} }: { open: boolean; onClose: () => void } = $props();
  const freeShippingThreshold = 300;
  const progress = $derived(Math.min(100, (cart.totalPrice / freeShippingThreshold) * 100));
</script>

{#if open}
  <div class="fixed inset-0 z-50">
    <button class="absolute inset-0 bg-black/70" onclick={onClose} aria-label="close cart"></button>
    <aside class="absolute right-0 top-0 h-full w-full max-w-md border-l border-zinc-300 bg-white p-4 text-black" transition:fly={{ x: 360, duration: 180 }}>
      <div class="mb-4 flex items-center justify-between">
        <h2 class="text-sm font-bold uppercase">Your Cart ({cart.totalItems})</h2>
        <button onclick={onClose}>Close</button>
      </div>
      <div class="mb-4 h-2 overflow-hidden rounded bg-zinc-200">
        <div class="h-full bg-accent" style={`width:${progress}%`}></div>
      </div>
      <p class="mb-4 text-xs text-zinc-600">
        {#if cart.totalPrice >= freeShippingThreshold}
          Free shipping unlocked.
        {:else}
          ${(freeShippingThreshold - cart.totalPrice).toFixed(2)} away from free shipping.
        {/if}
      </p>
      <div class="space-y-3">
        {#each cart.items as item (item.key)}
          <div class="rounded border border-zinc-300 p-3">
            <p class="text-xs font-semibold uppercase">{item.snapshot.title}</p>
            <p class="text-xs text-zinc-600">${item.snapshot.price.toFixed(2)}</p>
            <div class="mt-2 flex items-center gap-2">
              <button class="rounded bg-zinc-100 px-2" onclick={() => cart.updateQty(item.key, item.quantity - 1)}>-</button>
              <span class="text-xs">{item.quantity}</span>
              <button
                class="rounded bg-zinc-100 px-2 disabled:cursor-not-allowed disabled:opacity-50"
                onclick={() => cart.updateQty(item.key, item.quantity + 1)}
                disabled={item.snapshot.maxQuantity !== null && item.snapshot.maxQuantity !== undefined && item.quantity >= item.snapshot.maxQuantity}
              >
                +
              </button>
              <button class="ml-auto text-xs text-zinc-600" onclick={() => cart.remove(item.key)}>Remove</button>
            </div>
            {#if item.snapshot.maxQuantity !== null && item.snapshot.maxQuantity !== undefined}
              <p class="mt-2 text-[10px] uppercase tracking-wide text-zinc-500">Stock available: {item.snapshot.maxQuantity}</p>
            {/if}
          </div>
        {/each}
      </div>
      <div class="mt-6 border-t border-zinc-300 pt-4">
        <p class="mb-3 text-sm">Total: ${cart.totalPrice.toFixed(2)}</p>
        <Button class="w-full">Checkout</Button>
      </div>
    </aside>
  </div>
{/if}
