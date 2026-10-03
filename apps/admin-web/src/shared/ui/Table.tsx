import type { ReactNode } from "react";

export function Table({
  caption,
  head,
  children,
}: {
  caption: string;
  head: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="overflow-x-auto rounded-lg bg-white shadow-sm ring-1 ring-slate-200">
      <table className="min-w-full divide-y divide-slate-200 text-sm">
        <caption className="sr-only">{caption}</caption>
        <thead className="bg-slate-50 text-left text-xs font-semibold tracking-wide text-slate-600 uppercase">
          {head}
        </thead>
        <tbody className="divide-y divide-slate-100">{children}</tbody>
      </table>
    </div>
  );
}

export function Th({ children }: { children: ReactNode }) {
  return (
    <th scope="col" className="px-4 py-3">
      {children}
    </th>
  );
}

export function Td({ children, className }: { children: ReactNode; className?: string }) {
  return <td className={className ?? "px-4 py-3 align-top text-slate-800"}>{children}</td>;
}

export function EmptyRow({ colSpan, children }: { colSpan: number; children: ReactNode }) {
  return (
    <tr>
      <td colSpan={colSpan} className="px-4 py-8 text-center text-slate-500">
        {children}
      </td>
    </tr>
  );
}
