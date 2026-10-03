import type { ReactNode } from "react";

import { cx } from "./cx";

export type AlertKind = "info" | "error" | "success" | "warning";

const styles: Record<AlertKind, string> = {
  info: "bg-blue-50 text-blue-900 ring-blue-200",
  error: "bg-red-50 text-red-900 ring-red-200",
  success: "bg-green-50 text-green-900 ring-green-200",
  warning: "bg-amber-50 text-amber-900 ring-amber-200",
};

export function Alert({ kind = "info", children }: { kind?: AlertKind; children: ReactNode }) {
  return (
    <div
      role={kind === "error" ? "alert" : "status"}
      className={cx("rounded-md px-3 py-2 text-sm ring-1 ring-inset", styles[kind])}
    >
      {children}
    </div>
  );
}
