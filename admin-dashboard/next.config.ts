import type { NextConfig } from "next";

const nextConfig: NextConfig = {
  reactCompiler: true,
  // Smaller Docker image; server entry at .next/standalone (see Dockerfile).
  output: "standalone",
};

export default nextConfig;
