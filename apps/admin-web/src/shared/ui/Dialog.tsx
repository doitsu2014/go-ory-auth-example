import { type ReactNode, useEffect, useId, useRef } from "react";
import { useTranslation } from "react-i18next";

import { Button } from "./Button";
import { cx } from "./cx";

export interface ConfirmDialogProps {
  open: boolean;
  title: ReactNode;
  description?: ReactNode;
  children?: ReactNode;
  confirmLabel: ReactNode;
  danger?: boolean;
  busy?: boolean;
  /** When set, the dialog renders a <form> and calls this on submit. */
  onConfirm: () => void;
  onCancel: () => void;
  formId?: string;
  /** Omit the Cancel button (Escape still calls onCancel). */
  hideCancel?: boolean;
  /** Disable the confirm button (e.g. until an acknowledgement is ticked). */
  confirmDisabled?: boolean;
  /** Wider panel, for code samples. */
  wide?: boolean;
}

const FOCUSABLE =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

/**
 * Accessible modal confirmation dialog: role="dialog", aria-modal, labelled
 * by its title, focus moved inside and trapped, Escape cancels, focus restored
 * on close.
 */
export function ConfirmDialog({
  open,
  title,
  description,
  children,
  confirmLabel,
  danger,
  busy,
  onConfirm,
  onCancel,
  hideCancel,
  confirmDisabled,
  wide,
}: ConfirmDialogProps) {
  const { t } = useTranslation();
  const titleId = useId();
  const descId = useId();
  const panel = useRef<HTMLDivElement>(null);

  const cancelRef = useRef(onCancel);
  cancelRef.current = onCancel;
  const busyRef = useRef(busy);
  busyRef.current = busy;

  useEffect(() => {
    if (!open) return;
    const previous = document.activeElement as HTMLElement | null;
    const first = panel.current?.querySelector<HTMLElement>(FOCUSABLE);
    first?.focus();

    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.stopPropagation();
        if (!busyRef.current) cancelRef.current();
        return;
      }
      if (e.key !== "Tab" || !panel.current) return;
      const items = Array.from(panel.current.querySelectorAll<HTMLElement>(FOCUSABLE));
      const firstEl = items[0];
      const lastEl = items[items.length - 1];
      if (!firstEl || !lastEl) return;
      if (e.shiftKey && document.activeElement === firstEl) {
        e.preventDefault();
        lastEl.focus();
      } else if (!e.shiftKey && document.activeElement === lastEl) {
        e.preventDefault();
        firstEl.focus();
      }
    }
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("keydown", onKeyDown);
      previous?.focus();
    };
  }, [open]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-40 flex items-center justify-center bg-slate-900/40 p-4">
      <div
        ref={panel}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={description ? descId : undefined}
        tabIndex={-1}
        className={cx(
          "w-full rounded-lg bg-white p-6 shadow-xl",
          wide ? "max-h-full max-w-2xl overflow-y-auto" : "max-w-md",
        )}
      >
        <form
          noValidate
          onSubmit={(e) => {
            e.preventDefault();
            onConfirm();
          }}
          className="space-y-4"
        >
          <h2 id={titleId} className="text-lg font-semibold text-slate-900">
            {title}
          </h2>
          {description ? (
            <p id={descId} className="text-sm text-slate-600">
              {description}
            </p>
          ) : null}
          {children}
          <div className="flex justify-end gap-2 pt-2">
            {hideCancel ? null : (
              <Button variant="secondary" onClick={onCancel} disabled={busy}>
                {t("common.cancel")}
              </Button>
            )}
            <Button
              type="submit"
              variant={danger ? "danger" : "primary"}
              disabled={busy || confirmDisabled}
            >
              {confirmLabel}
            </Button>
          </div>
        </form>
      </div>
    </div>
  );
}
