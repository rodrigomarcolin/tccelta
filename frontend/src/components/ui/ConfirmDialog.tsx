"use client";

import { useEffect, useId, useRef, type ReactNode } from "react";
import { Button } from "./Button";
import { Note } from "./Note";

type Props = {
  open: boolean;
  title: string;
  children: ReactNode;
  confirmLabel: string;
  /** Label of the confirm button while the request runs. */
  pendingLabel: string;
  /** The request is running: both buttons are disabled and `Esc` does nothing. */
  pending?: boolean;
  /** Shown inside the dialog, which stays open. */
  error?: string | null;
  onConfirm: () => void;
  onCancel: () => void;
};

/**
 * Confirmation of a destructive action, on the native `<dialog>` opened with `showModal()`: the
 * browser traps the focus, makes the page behind inert, closes on `Esc` and gives the focus back
 * to the control that opened it. Focus starts on "Cancelar".
 */
export function ConfirmDialog({
  open,
  title,
  children,
  confirmLabel,
  pendingLabel,
  pending = false,
  error = null,
  onConfirm,
  onCancel,
}: Props) {
  const ref = useRef<HTMLDialogElement>(null);
  const cancelButton = useRef<HTMLButtonElement>(null);
  const titleId = useId();
  const textId = useId();

  useEffect(() => {
    const dialog = ref.current;
    if (!dialog) return;
    if (open && !dialog.open) {
      dialog.showModal();
      cancelButton.current?.focus();
    } else if (!open && dialog.open) {
      dialog.close();
    }
  }, [open]);

  // Closes with the component, so the page is never left inert.
  useEffect(() => {
    const dialog = ref.current;
    return () => {
      if (dialog?.open) dialog.close();
    };
  }, []);

  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      aria-describedby={textId}
      // `Esc`: the state is ours, so the browser does not close the dialog by itself.
      onCancel={(e) => {
        e.preventDefault();
        if (!pending) onCancel();
      }}
      // A click on the backdrop lands on the dialog element itself (the content fills the rest).
      onClick={(e) => {
        if (e.target === e.currentTarget && !pending) onCancel();
      }}
      className="m-auto w-[min(440px,calc(100%-32px))] rounded-lg border border-hairline bg-card p-0 text-primary backdrop:bg-black/60"
    >
      <div className="grid gap-4 p-6">
        <h2 id={titleId} className="m-0 type-title">
          {title}
        </h2>
        <p id={textId} className="m-0 text-secondary">
          {children}
        </p>
        {error ? (
          <Note role="alert" className="mt-0 text-danger">
            {error}
          </Note>
        ) : null}
        <div className="flex justify-end gap-3 phone:flex-col-reverse">
          <Button ref={cancelButton} variant="secondary" disabled={pending} onClick={onCancel}>
            Cancelar
          </Button>
          <Button variant="danger" disabled={pending} onClick={onConfirm}>
            {pending ? pendingLabel : confirmLabel}
          </Button>
        </div>
      </div>
    </dialog>
  );
}
