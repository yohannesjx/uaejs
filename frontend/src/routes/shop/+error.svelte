<script lang="ts">
  import { page } from "$app/state";
</script>

<section class="rounded-xl border border-zinc-800 bg-zinc-950 p-6">
  <h1 class="text-lg font-black uppercase">Shop failed to load</h1>
  <p class="mt-2 text-sm text-zinc-300">{page.error?.message || "Try refreshing this page."}</p>
  <a href="/shop" class="mt-4 inline-flex rounded border border-zinc-700 px-3 py-2 text-xs uppercase">Retry</a>
</section>
