<script lang="ts">
  import "../app.css";
  import { onNavigate } from "$app/navigation";
  import Navbar from "$lib/components/Navbar.svelte";
  import Footer from "$lib/components/Footer.svelte";

  onNavigate(() => {
    if (typeof document !== "undefined" && "startViewTransition" in document) {
      const anyDoc = document as Document & { startViewTransition?: (cb: () => void) => void };
      anyDoc.startViewTransition?.(() => {});
    }
  });
</script>

<svelte:head>
  <title>JS FASHION - Luxury Streetwear</title>
  <meta name="description" content="JS FASHION — luxury streetwear storefront." />
  <meta property="og:title" content="JS FASHION" />
  <meta property="og:description" content="Luxury streetwear shopping — JS FASHION." />
</svelte:head>

<div class="min-h-screen bg-bg text-fg flex flex-col">
  <Navbar />
  <main class="w-full px-4 pb-6 md:px-6 md:pb-8 xl:px-8 flex-1">
    <slot />
  </main>
  <Footer />
</div>
