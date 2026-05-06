# Production SvelteKit (adapter-node). Runtime is only Node + the `build/` output — no npm in the final image.
FROM node:22-alpine AS builder
WORKDIR /app

COPY package.json package-lock.json ./
RUN npm ci

COPY . .

# Optional: bake public API hints at build time (browser still infers origin when unset).
ARG PUBLIC_API_BASE_URL=
ARG PUBLIC_API_PORT=8080
ENV PUBLIC_API_BASE_URL=$PUBLIC_API_BASE_URL
ENV PUBLIC_API_PORT=$PUBLIC_API_PORT

RUN npx svelte-kit sync && npm run build

FROM node:22-alpine AS runner
WORKDIR /app

ENV NODE_ENV=production
ENV HOST=0.0.0.0
ENV PORT=3000

COPY --from=builder /app/build ./build

EXPOSE 3000
CMD ["node", "build/index.js"]
