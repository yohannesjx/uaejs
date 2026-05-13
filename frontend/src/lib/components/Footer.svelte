<script lang="ts">
  import { Instagram, Twitter, Facebook, Mail } from "lucide-svelte";
</script>

<footer class="mt-20 border-t border-zinc-200 bg-black text-white pt-16 pb-8 px-4 md:px-6 xl:px-8">
  <div class="mx-auto max-w-7xl">
    <!-- Newsletter Section -->
    <div class="grid lg:grid-cols-2 gap-12 lg:gap-24 border-b border-zinc-800 pb-16">
      <div class="max-w-md">
        <h2 class="text-2xl md:text-3xl font-black uppercase tracking-widest mb-4">Join The Club</h2>
        <p class="text-xs text-zinc-400 leading-relaxed mb-6">
          Subscribe to get exclusive access to limited drops, early sales, and insider-only updates.
        </p>
        <form class="flex border-b border-zinc-600 focus-within:border-white transition-colors" onsubmit={(e) => e.preventDefault()}>
          <input 
            type="email" 
            placeholder="ENTER YOUR EMAIL" 
            class="w-full bg-transparent py-3 text-xs uppercase tracking-widest outline-none placeholder:text-zinc-600"
            required
          />
          <button type="submit" class="px-4 text-xs font-bold uppercase tracking-widest hover:text-zinc-400 transition-colors">
            Subscribe
          </button>
        </form>
      </div>

      <!-- Links Section -->
      <div class="grid grid-cols-2 md:grid-cols-4 gap-8">
        <div class="flex flex-col space-y-4">
          <h3 class="text-[10px] font-bold uppercase tracking-widest text-zinc-500">Shop</h3>
          <a href="/shop" class="text-xs font-medium hover:text-zinc-400 transition-colors">Latest Drops</a>
          <a href="/shop" class="text-xs font-medium hover:text-zinc-400 transition-colors">Clothing</a>
          <a href="/shop" class="text-xs font-medium hover:text-zinc-400 transition-colors">Accessories</a>
          <a href="/shop" class="text-xs font-medium text-rose-500 hover:text-rose-400 transition-colors">Sale</a>
        </div>
        
        <div class="flex flex-col space-y-4">
          <h3 class="text-[10px] font-bold uppercase tracking-widest text-zinc-500">Support</h3>
          <a href="/faq" class="text-xs font-medium hover:text-zinc-400 transition-colors">FAQ</a>
          <a href="/shipping" class="text-xs font-medium hover:text-zinc-400 transition-colors">Shipping</a>
          <a href="/returns" class="text-xs font-medium hover:text-zinc-400 transition-colors">Returns</a>
          <a href="/contact" class="text-xs font-medium hover:text-zinc-400 transition-colors">Contact Us</a>
        </div>

        <div class="flex flex-col space-y-4">
          <h3 class="text-[10px] font-bold uppercase tracking-widest text-zinc-500">About</h3>
          <a href="/about" class="text-xs font-medium hover:text-zinc-400 transition-colors">Our Story</a>
          <a href="/careers" class="text-xs font-medium hover:text-zinc-400 transition-colors">Careers</a>
          <a href="/stockists" class="text-xs font-medium hover:text-zinc-400 transition-colors">Stockists</a>
        </div>

        <div class="flex flex-col space-y-4">
          <h3 class="text-[10px] font-bold uppercase tracking-widest text-zinc-500">Legal</h3>
          <a href="/terms" class="text-xs font-medium hover:text-zinc-400 transition-colors">Terms of Service</a>
          <a href="/privacy" class="text-xs font-medium hover:text-zinc-400 transition-colors">Privacy Policy</a>
          <a href="/cookies" class="text-xs font-medium hover:text-zinc-400 transition-colors">Cookie Policy</a>
        </div>
      </div>
    </div>

    <!-- Bottom Section -->
    <div class="pt-8 flex flex-col md:flex-row items-center justify-between gap-6">
      <a href="/" class="text-lg font-black uppercase tracking-[0.25em]">JS FASHION</a>
      
      <div class="flex items-center gap-6">
        <a href="https://instagram.com" class="text-zinc-400 hover:text-white transition-colors"><Instagram size={18} /></a>
        <a href="https://twitter.com" class="text-zinc-400 hover:text-white transition-colors"><Twitter size={18} /></a>
        <a href="https://facebook.com" class="text-zinc-400 hover:text-white transition-colors"><Facebook size={18} /></a>
        <a href="mailto:contact@noirdrop.com" class="text-zinc-400 hover:text-white transition-colors"><Mail size={18} /></a>
      </div>

      <div class="flex flex-col md:flex-row items-center gap-4 text-[10px] uppercase tracking-widest text-zinc-600">
        <button class="hover:text-zinc-300 transition-colors">United Arab Emirates (AED)</button>
        <span class="hidden md:inline">&copy; {new Date().getFullYear()} JS FASHION</span>
      </div>
      <!-- Mobile Copyright -->
      <span class="md:hidden text-[10px] uppercase tracking-widest text-zinc-600">&copy; {new Date().getFullYear()} JS FASHION</span>
    </div>
  </div>
</footer>
