<script lang="ts">
  import { page } from "$app/state";
</script>

<svelte:head>
  <title>Error | JS FASHION</title>
</svelte:head>

<section class="mx-auto max-w-xl rounded-xl border border-zinc-800 bg-zinc-950 p-6 text-center">
  <p class="text-xs uppercase tracking-wide text-accent">Something went wrong</p>
  <h1 class="mt-2 text-xl font-black uppercase">Page error</h1>
  <p class="mt-3 text-sm text-zinc-300">
    {page.error?.message || "Unexpected route error."}
  </p>
  <a href="/" class="mt-6 inline-flex rounded border border-zinc-700 px-4 py-2 text-xs uppercase">Go home</a>
</section>
