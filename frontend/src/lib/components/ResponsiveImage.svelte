<script lang="ts">
  let {
    src = "",
    alt = "",
    aspect = "aspect-[3/4]"
  }: { src?: string | null; alt?: string; aspect?: string } = $props();

  const fallback = "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='600' height='800'%3E%3Crect width='100%25' height='100%25' fill='%23111111'/%3E%3C/svg%3E";
</script>

<div class={`relative overflow-hidden bg-zinc-100 ${aspect}`}>
  <img
    src={src || fallback}
    alt={alt}
    loading="lazy"
    class="h-full w-full object-cover transition-transform duration-200 hover:scale-[1.02]"
    onerror={(e) => {
      (e.currentTarget as HTMLImageElement).src = fallback;
    }}
  />
</div>
