<script lang="ts">
  import type { Snippet } from "svelte";
  let {
    variant = "solid",
    class: className = "",
    children,
    ...rest
  }: {
    variant?: "solid" | "ghost" | "outline";
    class?: string;
    children?: Snippet;
    [key: string]: unknown;
  } = $props();

  const styles = {
    solid: "bg-accent text-white hover:scale-[1.01]",
    ghost: "bg-zinc-100 text-black hover:bg-zinc-200",
    outline: "border border-zinc-300 bg-transparent text-black hover:bg-zinc-100"
  };
</script>

<button
  {...rest}
  class={`inline-flex h-9 items-center justify-center rounded-md px-3 text-xs font-semibold uppercase tracking-wide transition-transform ${styles[variant]} ${className}`}
>
  {@render children?.()}
</button>
