<script lang="ts">
  import type { Snippet } from "svelte";
  import { fade, scale } from "svelte/transition";
  let {
    open = false,
    onClose = () => {},
    children
  }: {
    open: boolean;
    onClose: () => void;
    children?: Snippet;
  } = $props();
</script>

{#if open}
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4" transition:fade>
    <button class="absolute inset-0" aria-label="Close" onclick={onClose}></button>
    <div class="relative z-10 w-full max-w-lg rounded-xl border border-zinc-300 bg-white p-4 text-black" transition:scale>
      {@render children?.()}
    </div>
  </div>
{/if}
