import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatDate(value?: string | null) {
  if (!value) return "—";
  return new Intl.DateTimeFormat("en-AE", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

export function formatCurrency(
  value?: string | number | null,
  currency = "AED",
) {
  if (value === null || value === undefined || value === "") return "—";

  const numeric =
    typeof value === "number" ? value : Number.parseFloat(String(value));

  if (Number.isNaN(numeric)) return String(value);

  return new Intl.NumberFormat("en-AE", {
    style: "currency",
    currency,
    maximumFractionDigits: 2,
  }).format(numeric);
}

/** Amount for dense tables: two decimals, no currency code or symbol. */
export function formatAmountPlain(value?: string | number | null): string {
  if (value === null || value === undefined || value === "") return "—";
  const numeric = typeof value === "number" ? value : Number.parseFloat(String(value));
  if (Number.isNaN(numeric)) return String(value);
  return numeric.toFixed(2);
}

export function getInitials(value?: string | null) {
  if (!value) return "DU";
  return value
    .split(" ")
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0]?.toUpperCase() ?? "")
    .join("");
}
