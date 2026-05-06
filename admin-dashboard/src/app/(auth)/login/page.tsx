"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { AlertCircle, Lock, Mail } from "lucide-react";
import { useRouter } from "next/navigation";
import { useForm } from "react-hook-form";
import { z } from "zod";

import { Button, Card, CardContent, CardDescription, CardHeader, CardTitle, Input, Label } from "@/components/ui/primitives";
import { ApiError } from "@/lib/api-client";
import { useAuth } from "@/providers/auth-provider";

const loginSchema = z.object({
  email: z.string().email("Enter a valid email"),
  password: z.string().min(1, "Password is required"),
});

type LoginValues = z.infer<typeof loginSchema>;

export default function LoginPage() {
  const { login } = useAuth();
  const router = useRouter();
  const nextPath =
    typeof window !== "undefined"
      ? new URLSearchParams(window.location.search).get("next")
      : null;

  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
    setError,
  } = useForm<LoginValues>({
    resolver: zodResolver(loginSchema),
    defaultValues: {
      email: "",
      password: "",
    },
  });

  const onSubmit = async (values: LoginValues) => {
    try {
      await login(values.email, values.password);
      if (nextPath) {
        router.push(nextPath);
      }
    } catch (error) {
      setError("root", {
        message:
          error instanceof ApiError ? error.message : "Unable to sign in.",
      });
    }
  };

  return (
    <div className="grid min-h-screen bg-[radial-gradient(circle_at_top_left,_rgba(99,102,241,0.12),_transparent_28%),radial-gradient(circle_at_bottom_right,_rgba(236,72,153,0.1),_transparent_26%)] lg:grid-cols-[1.05fr_0.95fr]">
      <div className="hidden border-r border-[var(--border)] bg-[var(--sidebar)] p-10 lg:flex lg:flex-col lg:justify-between">
        <div>
          <p className="text-xs uppercase tracking-[0.24em] text-[var(--muted-foreground)]">
            Dubai Retail OS
          </p>
          <h1 className="mt-5 max-w-xl text-4xl font-semibold tracking-tight">
            Shopify-style operations control for fashion retail, wholesale, and warehouse execution.
          </h1>
          <p className="mt-4 max-w-xl text-base text-[var(--muted-foreground)]">
            Manage 6,000+ SKUs, stock transfers, loyalty, invoices, returns, and marketplace sync from one tenant-aware command center.
          </p>
        </div>

        <div className="grid grid-cols-2 gap-4">
          {[
            "Route-level RBAC",
            "Tenant-aware headers",
            "FIFO + COGS visibility",
            "Analytics + alerts",
          ].map((item) => (
            <Card key={item}>
              <CardContent className="p-4 text-sm text-[var(--muted-foreground)]">
                {item}
              </CardContent>
            </Card>
          ))}
        </div>
      </div>

      <div className="flex items-center justify-center p-6 lg:p-10">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle className="text-2xl">Sign in</CardTitle>
            <CardDescription>
              Use your admin account to access the tenant dashboard.
            </CardDescription>
          </CardHeader>

          <CardContent>
            <form className="space-y-4" onSubmit={handleSubmit(onSubmit)}>
              <div className="space-y-2">
                <Label htmlFor="email">Email</Label>
                <div className="relative">
                  <Mail className="pointer-events-none absolute left-3 top-3.5 size-4 text-[var(--muted-foreground)]" />
                  <Input
                    id="email"
                    type="email"
                    className="pl-10"
                    placeholder="admin@dubai-fashion.ae"
                    {...register("email")}
                  />
                </div>
                {errors.email ? (
                  <p className="text-xs text-rose-600">{errors.email.message}</p>
                ) : null}
              </div>

              <div className="space-y-2">
                <Label htmlFor="password">Password</Label>
                <div className="relative">
                  <Lock className="pointer-events-none absolute left-3 top-3.5 size-4 text-[var(--muted-foreground)]" />
                  <Input
                    id="password"
                    type="password"
                    className="pl-10"
                    placeholder="••••••••"
                    {...register("password")}
                  />
                </div>
                {errors.password ? (
                  <p className="text-xs text-rose-600">{errors.password.message}</p>
                ) : null}
              </div>

              {errors.root?.message ? (
                <div className="flex items-start gap-2 rounded-xl border border-rose-500/30 bg-rose-500/8 p-3 text-sm text-rose-700 dark:text-rose-300">
                  <AlertCircle className="mt-0.5 size-4 shrink-0" />
                  <span>{errors.root.message}</span>
                </div>
              ) : null}

              <Button type="submit" className="w-full" loading={isSubmitting}>
                Sign in to dashboard
              </Button>
            </form>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
