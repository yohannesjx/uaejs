import type { Config } from "tailwindcss";

export default {
  content: ["./src/**/*.{html,js,svelte,ts}"],
  theme: {
    extend: {
      colors: {
        bg: "#FFFFFF",
        fg: "#111111",
        accent: "#FF0066"
      }
    }
  },
  plugins: []
} satisfies Config;
