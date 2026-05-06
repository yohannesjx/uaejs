import { NextResponse, type NextRequest } from "next/server";

const protectedRoutes = [
  "/dashboard",
  "/products",
  "/orders",
  "/warehouses",
  "/customers",
  "/returns",
  "/analytics",
  "/users",
  "/settings",
  "/channels",
  "/suppliers",
  "/shipments",
];

export function proxy(request: NextRequest) {
  const accessToken = request.cookies.get("access_token")?.value;
  const { pathname } = request.nextUrl;

  const isProtected = protectedRoutes.some(
    (route) => pathname === route || pathname.startsWith(`${route}/`),
  );

  if (isProtected && !accessToken) {
    const loginUrl = new URL("/login", request.url);
    loginUrl.searchParams.set("next", pathname);
    return NextResponse.redirect(loginUrl);
  }

  if (pathname === "/login" && accessToken) {
    return NextResponse.redirect(new URL("/dashboard", request.url));
  }

  return NextResponse.next();
}

export const config = {
  matcher: [
    "/dashboard/:path*",
    "/products/:path*",
    "/orders/:path*",
    "/warehouses/:path*",
    "/customers/:path*",
    "/returns/:path*",
    "/analytics/:path*",
    "/users/:path*",
    "/settings/:path*",
    "/channels/:path*",
    "/suppliers/:path*",
    "/shipments/:path*",
    "/login",
  ],
};
