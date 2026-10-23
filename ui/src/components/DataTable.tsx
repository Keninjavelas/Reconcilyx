import type { ReactNode } from "react";
import clsx from "clsx";

export interface Column<T> {
  key: keyof T | string;
  title: ReactNode;
  className?: string;
  render?: (row: T) => ReactNode;
  sortable?: boolean;
}

interface DataTableProps<T> {
  columns: Column<T>[];
  rows: T[];
  empty?: ReactNode;
  rowKey?: (row: T, idx: number) => string;
  onRowClick?: (row: T) => void;
  className?: string;
}

export default function DataTable<T>({
  columns,
  rows,
  empty,
  rowKey,
  onRowClick,
  className,
}: DataTableProps<T>) {
  return (
    <div className={clsx("overflow-x-auto scrollbar-thin", className)}>
      <table className="data-table">
        <thead>
          <tr>
            {columns.map((c) => (
              <th key={String(c.key)} className={c.className}>
                {c.title}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <tr>
              <td colSpan={columns.length} className="!py-14 text-center text-[13px] text-ink-400">
                {empty ?? "No records."}
              </td>
            </tr>
          ) : (
            rows.map((r, i) => (
              <tr
                key={rowKey ? rowKey(r, i) : i}
                onClick={onRowClick ? () => onRowClick(r) : undefined}
                className={clsx(onRowClick && "cursor-pointer")}
              >
                {columns.map((c) => {
                  const val =
                    typeof c.key === "string" ? (r as Record<string, unknown>)[c.key] : undefined;
                  return (
                    <td key={String(c.key)} className={c.className}>
                      {c.render ? c.render(r) : (val as ReactNode)}
                    </td>
                  );
                })}
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  );
}
