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
  <title>Noir Drop - Luxury Streetwear</title>
  <meta name="description" content="High-conversion luxury streetwear storefront." />
  <meta property="og:title" content="Noir Drop" />
  <meta property="og:description" content="Black, white, pink streetwear shopping experience." />
</svelte:head>

<div class="min-h-screen bg-bg text-fg flex flex-col">
  <Navbar />
  <main class="w-full px-4 pb-6 md:px-6 md:pb-8 xl:px-8 flex-1">
    <slot />
  </main>
  <Footer />
</div>
