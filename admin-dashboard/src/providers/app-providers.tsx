"use client";

import { AuthProvider } from "@/providers/auth-provider";
import { AppQueryProvider } from "@/providers/query-provider";
import { ThemeProvider } from "@/providers/theme-provider";

export function AppProviders({ children }: { children: React.ReactNode }) {
  return (
    <ThemeProvider>
      <AppQueryProvider>
        <AuthProvider>{children}</AuthProvider>
      </AppQueryProvider>
    </ThemeProvider>
  );
}
