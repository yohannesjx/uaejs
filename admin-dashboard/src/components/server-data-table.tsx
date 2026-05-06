"use client";

import {
  flexRender,
  getCoreRowModel,
  useReactTable,
  type ColumnDef,
} from "@tanstack/react-table";
import { useCallback, useEffect, useState } from "react";

import { Button, Input } from "@/components/ui/primitives";
import { cn } from "@/lib/utils";

interface ServerDataTableProps<TData> {
  data: TData[];
  total: number;
  page: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onSearchChange?: (search: string) => void;
  searchPlaceholder?: string;
  searchDebounceMs?: number;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  columns: ColumnDef<TData, any>[];
  className?: string;
  isLoading?: boolean;
}

export function ServerDataTable<TData>({
  data,
  total,
  page,
  pageSize,
  onPageChange,
  onSearchChange,
  searchPlaceholder = "Search...",
  searchDebounceMs = 300,
  columns,
  className,
  isLoading,
}: ServerDataTableProps<TData>) {
  const [searchInput, setSearchInput] = useState("");

  useEffect(() => {
    if (onSearchChange == null) return;
    const t = setTimeout(() => {
      onSearchChange(searchInput);
    }, searchDebounceMs);
    return () => clearTimeout(t);
  }, [searchInput, searchDebounceMs, onSearchChange]);

  const table = useReactTable({
    data,
    columns,
    getCoreRowModel: getCoreRowModel(),
  });

  const rows = table.getRowModel().rows;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const canPrevious = page > 1;
  const canNext = page < totalPages;
  const from = total === 0 ? 0 : (page - 1) * pageSize + 1;
  const to = Math.min(page * pageSize, total);

  const handlePrevious = useCallback(() => {
    if (canPrevious) onPageChange(page - 1);
  }, [canPrevious, page, onPageChange]);

  const handleNext = useCallback(() => {
    if (canNext) onPageChange(page + 1);
  }, [canNext, page, onPageChange]);

  return (
    <div className={cn("space-y-4", className)}>
      {onSearchChange != null && (
        <Input
          value={searchInput}
          onChange={(e) => setSearchInput(e.target.value)}
          placeholder={searchPlaceholder}
        />
      )}

      <div className="max-h-[520px] overflow-auto rounded-2xl border border-[var(--border)]">
        <table className="min-w-full text-left text-sm">
          <thead className="sticky top-0 z-10 bg-[var(--panel)] backdrop-blur">
            {table.getHeaderGroups().map((headerGroup) => (
              <tr key={headerGroup.id} className="border-b border-[var(--border)]">
                {headerGroup.headers.map((header) => (
                  <th
                    key={header.id}
                    className="px-4 py-3 font-medium text-[var(--muted-foreground)]"
                  >
                    {header.isPlaceholder
                      ? null
                      : flexRender(
                          header.column.columnDef.header,
                          header.getContext(),
                        )}
                  </th>
                ))}
              </tr>
            ))}
          </thead>
          <tbody>
            {isLoading ? (
              <tr>
                <td
                  colSpan={columns.length}
                  className="px-4 py-12 text-center text-[var(--muted-foreground)]"
                >
                  Loading...
                </td>
              </tr>
            ) : rows.length === 0 ? (
              <tr>
                <td
                  colSpan={columns.length}
                  className="px-4 py-12 text-center text-[var(--muted-foreground)]"
                >
                  No results
                </td>
              </tr>
            ) : (
              rows.map((row) => (
                <tr
                  key={row.id}
                  className="border-b border-[var(--border)] transition hover:bg-[var(--muted)]/60"
                >
                  {row.getVisibleCells().map((cell) => (
                    <td key={cell.id} className="px-4 py-3 align-top">
                      {flexRender(
                        cell.column.columnDef.cell,
                        cell.getContext(),
                      )}
                    </td>
                  ))}
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>

      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-[var(--muted-foreground)]">
          {total === 0
            ? "0 results"
            : `${from}–${to} of ${total} result${total === 1 ? "" : "s"}`}
        </p>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={handlePrevious}
            disabled={!canPrevious || isLoading}
          >
            Previous
          </Button>
          <span className="text-sm text-[var(--muted-foreground)]">
            Page {page} of {totalPages}
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={handleNext}
            disabled={!canNext || isLoading}
          >
            Next
          </Button>
        </div>
      </div>
    </div>
  );
}
