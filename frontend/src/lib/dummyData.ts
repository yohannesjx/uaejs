const categories = ["streetwear", "oversized", "minimal", "accessories"] as const;
const colors = ["Black", "White", "Charcoal", "Pink", "Olive", "Sand"];
const sizes = ["XS", "S", "M", "L", "XL"];

function product(idx: number) {
  const category = categories[idx % categories.length];
  const color = colors[idx % colors.length];
  const title = `${category.toUpperCase()} DROP ${idx + 1}`;
  return {
    id: `p-${idx + 1}`,
    slug: `drop-${idx + 1}`,
    title,
    price: 120 + idx * 7,
    compare_at_price: idx % 3 === 0 ? 160 + idx * 5 : null,
    image_url: `https://picsum.photos/seed/fashion-${idx + 1}/900/1200`,
    images: [
      `https://picsum.photos/seed/fashion-${idx + 1}/900/1200`,
      `https://picsum.photos/seed/fashion-${idx + 101}/900/1200`
    ],
    category,
    color,
    sizes: sizes.slice(0, 2 + (idx % 4)),
    tags: [category, "drop", idx % 2 === 0 ? "new" : "core"],
    description: "Premium cut. Minimal silhouette. Built for everyday rotation.",
    inventory: Math.max(0, 18 - idx)
  };
}

export const dummyProducts = Array.from({ length: 200 }, (_, i) => product(i));
