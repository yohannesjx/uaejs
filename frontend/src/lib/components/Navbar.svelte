<script lang="ts">
  import { ShoppingBag, Search, User, Heart, Menu } from "lucide-svelte";
  import CartDrawer from "./CartDrawer.svelte";
  import { cart } from "$lib/state/cart.svelte";

  let drawerOpen = $state(false);
  let searchOpen = $state(false);
  let scrollDirection = $state<"up" | "down">("up");
  let isVisible = $state(true);
  let hasScrolled = $state(false);
  let currentScrollY = $state(0);
  let lastScrollY = $state(0);

  const TOP_LOCK_VISIBLE = 50;
  const DELTA_THRESHOLD = 6;

  const menuItems = ["Shop", "Men", "Women", "Accessories", "Drops", "Sale"];

  $effect(() => {
    // Initialize scroll position
    lastScrollY = window.scrollY || document.documentElement.scrollTop;
    let accDown = 0;
    let accUp = 0;

    const onScroll = () => {
      currentScrollY = window.scrollY || document.documentElement.scrollTop;
      hasScrolled = currentScrollY > 4;

      if (currentScrollY <= TOP_LOCK_VISIBLE) {
        isVisible = true;
        scrollDirection = "up";
        accDown = 0;
        accUp = 0;
        lastScrollY = currentScrollY;
        return;
      }

      const delta = currentScrollY - lastScrollY;

      if (delta > 0) {
        accDown += delta;
        accUp = 0;
        if (accDown >= DELTA_THRESHOLD) {
          scrollDirection = "down";
          isVisible = false;
          accDown = 0;
        }
      } else if (delta < 0) {
        accUp += Math.abs(delta);
        accDown = 0;
        if (accUp >= DELTA_THRESHOLD) {
          scrollDirection = "up";
          isVisible = true;
          accUp = 0;
        }
      }

      lastScrollY = currentScrollY;
    };

    window.addEventListener("scroll", onScroll, { passive: true });
    
    return () => {
      window.removeEventListener("scroll", onScroll);
    };
  });
</script>

<header
  class="fixed left-0 right-0 top-0 z-[90] transform-gpu will-change-transform transition-all duration-300 ease-in-out {isVisible ? 'nav-visible' : 'nav-hidden'} {hasScrolled || searchOpen ? 'bg-black/90 backdrop-blur-md border-b border-zinc-800' : 'bg-transparent border-b border-transparent'}"
  data-direction={scrollDirection}
>
  <div class="w-full px-4 py-3 md:px-6 xl:px-8">
    <!-- Desktop & Mobile Top Bar (Unified Layout) -->
    <div class="flex items-center justify-between w-full">
      
      <!-- Left: Menu & Heart -->
      <div class="flex items-center gap-4 flex-1">
        <button class="text-zinc-300 hover:text-white transition-colors" aria-label="Menu"><Menu size={20} /></button>
        <button class="text-zinc-300 hover:text-white transition-colors" aria-label="Wishlist"><Heart size={18} /></button>
      </div>
      
      <!-- Center: Logo -->
      <div class="flex-1 text-center">
        <a href="/" class="text-base md:text-lg font-black uppercase tracking-[0.25em] text-white">JS FASHION</a>
      </div>
      
      <!-- Right: Search & Cart -->
      <div class="flex items-center justify-end gap-5 text-zinc-300 flex-1">
        <button class="hover:text-white transition-colors" aria-label="Search" onclick={() => searchOpen = !searchOpen}>
          <Search size={18} />
        </button>
        <button class="relative flex items-center gap-1.5 hover:text-white transition-colors" aria-label="Cart" onclick={() => (drawerOpen = true)}>
          <ShoppingBag size={18} />
          {#if cart.totalItems > 0}
            <span class="absolute -top-1.5 -right-2 flex h-4 w-4 items-center justify-center rounded-full bg-accent text-[9px] font-bold text-white">
              {cart.totalItems}
            </span>
          {/if}
        </button>
      </div>

    </div>
  </div>

  <!-- Toggleable Search Box -->
  {#if searchOpen}
    <div class="w-full px-4 pb-4 pt-2 md:px-6 xl:px-8 border-t border-zinc-800">
      <div class="relative max-w-2xl mx-auto">
        <Search size={16} class="absolute left-4 top-1/2 -translate-y-1/2 text-zinc-400" />
        <input class="h-10 w-full rounded-full bg-zinc-900/80 pl-11 pr-4 text-xs text-white placeholder-zinc-500 outline-none focus:bg-zinc-800 transition-colors" placeholder="Search for products, brands, or styles..." autofocus />
      </div>
    </div>
  {/if}
</header>

<CartDrawer open={drawerOpen} onClose={() => (drawerOpen = false)} />

<style>
  .nav-visible {
    transform: translateY(0%);
  }
  .nav-hidden {
    transform: translateY(-100%);
  }
  
  /* Hide scrollbar for mobile nav */
  .hide-scrollbar::-webkit-scrollbar {
    display: none;
  }
  .hide-scrollbar {
    -ms-overflow-style: none;
    scrollbar-width: none;
  }
</style>
