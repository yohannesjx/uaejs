# Dubai Retail OS Admin Dashboard

Modern Next.js admin dashboard for the Dubai Retail OS backend.

## Stack

- Next.js App Router
- TypeScript
- Tailwind CSS
- TanStack Query
- TanStack Table
- React Hook Form + Zod
- Radix UI primitives
- Recharts
- `cmd+k` command palette
- Tenant-aware API client with JWT refresh flow

## Run locally

```bash
cd admin-dashboard
cp .env.example .env.local
npm install
npm run dev
```

Default API target:

```env
NEXT_PUBLIC_API_BASE_URL=http://localhost:8080
```

## Current backend-integrated sections

- `login` with JWT + refresh-token flow
- `dashboard` analytics overview
- `users` with RBAC-aware access
- `warehouses` and warehouse inventory detail
- `customers` and customer loyalty profile detail
- `channels`
- `suppliers`
- `settings` with tenant settings persistence
- `returns` detail lookup
- `shipments` tracking lookup
- `products` create flow
- `orders` invoice XML lookup

## Known backend surface gaps

Some dashboard sections are intentionally scaffolded around the current REST surface:

- Products: create + detail exist, but list/search endpoints are not exposed yet
- Orders: create + detail + invoice XML exist, but list/filter endpoints are not exposed yet
- Customers: create + detail + loyalty endpoints exist, but list/search endpoints are not exposed yet
- Returns: detail exists, but list endpoint is not exposed yet
- Shipments: detail/tracking exist, but list endpoint is not exposed yet

The frontend is structured so those routes can upgrade to full TanStack Table grids with minimal refactoring when the backend list APIs are added.
This is a [Next.js](https://nextjs.org) project bootstrapped with [`create-next-app`](https://nextjs.org/docs/app/api-reference/cli/create-next-app).

## Getting Started

First, run the development server:

```bash
npm run dev
# or
yarn dev
# or
pnpm dev
# or
bun dev
```

Open [http://localhost:3000](http://localhost:3000) with your browser to see the result.

You can start editing the page by modifying `app/page.tsx`. The page auto-updates as you edit the file.

This project uses [`next/font`](https://nextjs.org/docs/app/building-your-application/optimizing/fonts) to automatically optimize and load [Geist](https://vercel.com/font), a new font family for Vercel.

## Learn More

To learn more about Next.js, take a look at the following resources:

- [Next.js Documentation](https://nextjs.org/docs) - learn about Next.js features and API.
- [Learn Next.js](https://nextjs.org/learn) - an interactive Next.js tutorial.

You can check out [the Next.js GitHub repository](https://github.com/vercel/next.js) - your feedback and contributions are welcome!

## Deploy on Vercel

The easiest way to deploy your Next.js app is to use the [Vercel Platform](https://vercel.com/new?utm_medium=default-template&filter=next.js&utm_source=create-next-app&utm_campaign=create-next-app-readme) from the creators of Next.js.

Check out our [Next.js deployment documentation](https://nextjs.org/docs/app/building-your-application/deploying) for more details.
