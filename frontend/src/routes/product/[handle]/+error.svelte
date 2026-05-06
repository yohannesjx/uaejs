<script lang="ts">
  import { page } from "$app/state";
</script>

<section class="rounded-xl border border-zinc-800 bg-zinc-950 p-6">
  <h1 class="text-lg font-black uppercase">Product failed to load</h1>
  <p class="mt-2 text-sm text-zinc-300">{page.error?.message || "The product route encountered an error."}</p>
  <a href="/shop" class="mt-4 inline-flex rounded border border-zinc-700 px-3 py-2 text-xs uppercase">Back to shop</a>
</section>
