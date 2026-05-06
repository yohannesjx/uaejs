"use client";

import {
  flexRender,
  getCoreRowModel,
  getFilteredRowModel,
  getPaginationRowModel,
  getSortedRowModel,
  useReactTable,
  type ColumnDef,
  type SortingState,
} from "@tanstack/react-table";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useMemo, useRef, useState } from "react";

import { Button, Input } from "@/components/ui/primitives";
import { cn } from "@/lib/utils";

interface DataTableProps<TData> {
  data: TData[];
  // TanStack Table column definitions intentionally carry heterogeneous cell
  // value types across one table instance, so `any` is the practical shared
  // boundary for a generic admin-grid component.
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  columns: ColumnDef<TData, any>[];
  searchPlaceholder?: string;
  className?: string;
}

export function DataTable<TData>({
  data,
  columns,
  searchPlaceholder = "Search...",
  className,
}: DataTableProps<TData>) {
  const [sorting, setSorting] = useState<SortingState>([]);
  const [globalFilter, setGlobalFilter] = useState("");
  const tableContainerRef = useRef<HTMLDivElement | null>(null);

  const table = useReactTable({
    data,
    columns,
    state: { sorting, globalFilter },
    onSortingChange: setSorting,
    onGlobalFilterChange: setGlobalFilter,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    getFilteredRowModel: getFilteredRowModel(),
    getPaginationRowModel: getPaginationRowModel(),
  });

  const rows = table.getRowModel().rows;

  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => tableContainerRef.current,
    estimateSize: () => 56,
    overscan: 8,
  });

  const virtualRows = rowVirtualizer.getVirtualItems();
  const totalSize = rowVirtualizer.getTotalSize();
  const paddingTop = virtualRows.length > 0 ? virtualRows[0]?.start ?? 0 : 0;
  const paddingBottom =
    virtualRows.length > 0
      ? totalSize - (virtualRows[virtualRows.length - 1]?.end ?? 0)
      : 0;

  const pagedRows = useMemo(
    () => virtualRows.map((virtualRow) => rows[virtualRow.index]),
    [rows, virtualRows],
  );

  return (
    <div className={cn("space-y-4", className)}>
      <Input
        value={globalFilter}
        onChange={(event) => setGlobalFilter(event.target.value)}
        placeholder={searchPlaceholder}
      />

      <div
        ref={tableContainerRef}
        className="max-h-[520px] overflow-auto rounded-2xl border border-[var(--border)]"
      >
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
            {paddingTop > 0 ? (
              <tr>
                <td colSpan={columns.length} style={{ height: paddingTop }} />
              </tr>
            ) : null}

            {pagedRows.map((row) => (
              <tr
                key={row.id}
                className="border-b border-[var(--border)] transition hover:bg-[var(--muted)]/60"
              >
                {row.getVisibleCells().map((cell) => (
                  <td key={cell.id} className="px-4 py-3 align-top">
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </td>
                ))}
              </tr>
            ))}

            {paddingBottom > 0 ? (
              <tr>
                <td colSpan={columns.length} style={{ height: paddingBottom }} />
              </tr>
            ) : null}
          </tbody>
        </table>
      </div>

      <div className="flex items-center justify-between gap-3">
        <p className="text-sm text-[var(--muted-foreground)]">
          {rows.length} result{rows.length === 1 ? "" : "s"}
        </p>
        <div className="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => table.previousPage()}
            disabled={!table.getCanPreviousPage()}
          >
            Previous
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => table.nextPage()}
            disabled={!table.getCanNextPage()}
          >
            Next
          </Button>
        </div>
      </div>
    </div>
  );
}
