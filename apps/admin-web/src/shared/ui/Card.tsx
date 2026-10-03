import type { ReactNode } from "react";

import { cx } from "./cx";

export function Card({ children, className }: { children: ReactNode; className?: string }) {
  return (
    <section className={cx("rounded-lg bg-white p-6 shadow-sm ring-1 ring-slate-200", className)}>
      {children}
    </section>
  );
}

export function PageHeader({ title, actions }: { title: ReactNode; actions?: ReactNode }) {
  return (
    <div className="mb-6 flex flex-wrap items-center justify-between gap-3">
      <h1 className="text-2xl font-semibold text-slate-900">{title}</h1>
      {actions}
    </div>
  );
}
