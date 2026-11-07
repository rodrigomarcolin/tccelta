"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { ApiError } from "@/domain/trip/apiError";
import { useDeleteTrip } from "@/use-cases/useDeleteTrip";
import { ConfirmDialog } from "@/components/ui/ConfirmDialog";

type Props = {
  open: boolean;
  tripId: string;
  /** The driver of the trip, when known. */
  driverName: string | null;
  /** Where the login sends the operator back to, if the session has expired. */
  returnTo: string;
  /** Cancelled, by the button, `Esc` or the backdrop. */
  onClose: () => void;
  /** The deletion went through (or the trip was already gone). */
  onDeleted: () => void;
  /** The request starts (`true`) or fails (`false`); after a success it stays `true`. */
  onDeleting?: (deleting: boolean) => void;
};

/** Confirmation and request to delete a trip; shared by the list and the analysis page. */
export function DeleteTripDialog({
  open,
  tripId,
  driverName,
  returnTo,
  onClose,
  onDeleted,
  onDeleting,
}: Props) {
  const router = useRouter();
  const mutation = useDeleteTrip();
  const { reset } = mutation;
  // Closed (cancelled, or deleted from the list): the next trip starts from a clean state.
  useEffect(() => {
    if (!open) reset();
  }, [open, reset]);

  const confirm = () => {
    onDeleting?.(true);
    mutation.mutate(tripId, {
      onSuccess: onDeleted,
      onError: (error) => {
        onDeleting?.(false);
        if (error instanceof ApiError && error.status === 401) {
          router.replace(`/entrar?retorno=${encodeURIComponent(returnTo)}`);
        }
      },
    });
  };
  const cancel = () => {
    reset();
    onClose();
  };

  return (
    <ConfirmDialog
      open={open}
      title={driverName ? `Excluir a viagem de ${driverName}?` : "Excluir esta viagem?"}
      confirmLabel="Excluir"
      pendingLabel="Excluindo…"
      // After a success the dialog keeps saying "Excluindo…" until the page leaves.
      pending={mutation.isPending || mutation.isSuccess}
      error={mutation.isError ? "Não foi possível excluir. Tente de novo." : null}
      onConfirm={confirm}
      onCancel={cancel}
    >
      Ela some da lista e da análise, e o nome e o boot ficam livres para uma nova viagem. Não é
      possível desfazer.
    </ConfirmDialog>
  );
}
