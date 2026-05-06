"use client";

import * as AvatarPrimitive from "@radix-ui/react-avatar";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { Loader2 } from "lucide-react";
import * as React from "react";

import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-lg text-xs font-medium transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--ring)] disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        default:
          "border border-[var(--border)] bg-[var(--muted)] text-[var(--foreground)] hover:bg-[color:color-mix(in_srgb,var(--muted)_78%,black_22%)]",
        outline:
          "border border-[var(--border)] bg-transparent text-[var(--foreground)] hover:bg-[var(--muted)]",
        ghost: "text-[var(--foreground)] hover:bg-[var(--muted)]",
        danger: "bg-[var(--danger)] text-white hover:opacity-90",
      },
      size: {
        default: "h-10 md:h-9 px-3.5 py-1.5",
        sm: "h-8 md:h-7 rounded-md px-2.5 text-[11px]",
        lg: "h-11 md:h-10 px-4 text-xs md:text-sm",
        icon: "h-10 w-10 md:h-8 md:w-8",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
  VariantProps<typeof buttonVariants> {
  asChild?: boolean;
  loading?: boolean;
}

export function Button({
  className,
  variant,
  size,
  asChild,
  loading,
  children,
  ...props
}: ButtonProps) {
  const Comp = asChild ? Slot : "button";

  return (
    <Comp className={cn(buttonVariants({ variant, size, className }))} {...props}>
      {asChild ? children : (
        <>
          {loading ? <Loader2 className="size-4 animate-spin" /> : null}
          {children}
        </>
      )}
    </Comp>
  );
}

export function Input({
  className,
  ...props
}: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={cn(
        "flex h-10 md:h-9 w-full rounded-lg border border-[var(--border)] bg-[var(--panel)] px-3 md:px-2.5 py-1.5 text-sm md:text-xs text-[var(--foreground)] outline-none transition placeholder:text-[var(--muted-foreground)] focus:border-[var(--ring)] focus:ring-2 focus:ring-[var(--ring)]/30",
        className,
      )}
      {...props}
    />
  );
}

export function Textarea({
  className,
  ...props
}: React.TextareaHTMLAttributes<HTMLTextAreaElement>) {
  return (
    <textarea
      className={cn(
        "flex min-h-[5rem] w-full rounded-lg border border-[var(--border)] bg-[var(--panel)] px-3 md:px-2.5 py-2 text-sm md:text-xs text-[var(--foreground)] outline-none transition placeholder:text-[var(--muted-foreground)] focus:border-[var(--ring)] focus:ring-2 focus:ring-[var(--ring)]/30",
        className,
      )}
      {...props}
    />
  );
}

export function Label({
  className,
  ...props
}: React.LabelHTMLAttributes<HTMLLabelElement>) {
  return (
    <label
      className={cn("text-xs font-medium text-[var(--foreground)]", className)}
      {...props}
    />
  );
}

export function Card({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "rounded-xl border border-[var(--border)] bg-[var(--panel)] shadow-sm transition-shadow hover:shadow-md",
        className,
      )}
      {...props}
    />
  );
}

export function CardHeader({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("p-4 pb-0", className)} {...props} />;
}

export function CardTitle({
  className,
  ...props
}: React.HTMLAttributes<HTMLHeadingElement>) {
  return <h3 className={cn("text-sm font-semibold", className)} {...props} />;
}

export function CardDescription({
  className,
  ...props
}: React.HTMLAttributes<HTMLParagraphElement>) {
  return (
    <p className={cn("mt-1 text-xs text-[var(--muted-foreground)]", className)} {...props} />
  );
}

export function CardContent({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("p-4", className)} {...props} />;
}

export function Badge({
  className,
  tone = "default",
  ...props
}: React.HTMLAttributes<HTMLSpanElement> & {
  tone?: "default" | "success" | "warning" | "danger";
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2.5 py-1 text-xs font-medium",
        tone === "default" &&
        "bg-[color:color-mix(in_srgb,var(--muted)_82%,black_18%)] text-[var(--foreground)]",
        tone === "success" &&
        "bg-emerald-500/12 text-emerald-600 dark:text-emerald-400",
        tone === "warning" &&
        "bg-amber-500/12 text-amber-700 dark:text-amber-300",
        tone === "danger" && "bg-rose-500/12 text-rose-700 dark:text-rose-300",
        className,
      )}
      {...props}
    />
  );
}

export function Skeleton({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn("animate-pulse rounded-xl bg-[var(--muted)]", className)}
      {...props}
    />
  );
}

export function Separator({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("h-px w-full bg-[var(--border)]", className)} {...props} />;
}

export function Avatar({
  name,
  className,
}: {
  name?: string | null;
  className?: string;
}) {
  const initials =
    name
      ?.split(" ")
      .filter(Boolean)
      .slice(0, 2)
      .map((part) => part[0]?.toUpperCase() ?? "")
      .join("") || "DU";

  return (
    <AvatarPrimitive.Root
      className={cn(
        "relative flex size-8 shrink-0 overflow-hidden rounded-full border border-[var(--border)] bg-[var(--muted)]",
        className,
      )}
    >
      <AvatarPrimitive.Fallback className="flex h-full w-full items-center justify-center text-[11px] font-semibold text-[var(--foreground)]">
        {initials}
      </AvatarPrimitive.Fallback>
    </AvatarPrimitive.Root>
  );
}

