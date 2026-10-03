import type { ButtonHTMLAttributes } from "react";

import { cx } from "./cx";

type Variant = "primary" | "secondary" | "danger" | "ghost";

const variants: Record<Variant, string> = {
  primary: "bg-blue-700 text-white hover:bg-blue-800 disabled:bg-blue-300",
  secondary:
    "bg-white text-slate-800 ring-1 ring-inset ring-slate-300 hover:bg-slate-100 disabled:text-slate-400",
  danger: "bg-red-700 text-white hover:bg-red-800 disabled:bg-red-300",
  ghost: "text-slate-700 hover:bg-slate-100 disabled:text-slate-400",
};

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: Variant;
}

export function Button({ variant = "primary", className, type = "button", ...rest }: ButtonProps) {
  return (
    <button
      type={type}
      className={cx(
        "inline-flex items-center justify-center gap-2 rounded-md px-3 py-2 text-sm font-medium transition-colors disabled:cursor-not-allowed",
        variants[variant],
        className,
      )}
      {...rest}
    />
  );
}
