"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { IconTrash } from "@/components/icons/Icons";
import { Button } from "@/components/ui/Button";
import { DeleteTripDialog } from "./DeleteTripDialog";

type Props = {
  tripId: string;
  driverName: string | null;
  /** Reports that the request is under way, so the page can stop polling the trip. */
  onDeleting?: (deleting: boolean) => void;
};

/** "Excluir viagem" on the analysis page: confirms, deletes and goes back to the list. */
export function DeleteTripAction({ tripId, driverName, onDeleting }: Props) {
  const router = useRouter();
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button
        variant="secondary"
        icon={<IconTrash />}
        className="print:hidden"
        onClick={() => setOpen(true)}
      >
        Excluir viagem
      </Button>
      <DeleteTripDialog
        open={open}
        tripId={tripId}
        driverName={driverName}
        returnTo={`/viagem/${tripId}`}
        onClose={() => setOpen(false)}
        onDeleting={onDeleting}
        onDeleted={() => router.replace("/viagens")}
      />
    </>
  );
}
