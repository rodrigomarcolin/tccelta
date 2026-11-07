"use client";

import { useMemo, useState, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { useTripMonitor } from "@/use-cases/useTripMonitor";
import { HeaderControls } from "@/components/ui/AppNav";
import { PageHeader } from "@/components/ui/PageHeader";
import { DeleteTripAction } from "@/components/trips/DeleteTripAction";
import { AnalysisView } from "./AnalysisView";
import {
  LoadingState,
  ErrorState,
  WaitingState,
  FailedState,
  WaitLimitState,
  NotFoundState,
  ConnectionBanner,
} from "./States";

type Props = { id: string };

function SimplePage({
  children,
  text,
  actions,
}: {
  children: ReactNode;
  text?: string | null;
  /** Page actions (delete), above the state card. */
  actions?: ReactNode;
}) {
  return (
    <div>
      <PageHeader>
        <h1>Análise de viagem</h1>
        <HeaderControls />
      </PageHeader>
      {text ? <ConnectionBanner text={text} /> : null}
      {actions ? <div className="mb-5 flex print:hidden">{actions}</div> : null}
      {children}
    </div>
  );
}

/** Waiting, polling and display of the analysis for `/viagem/[id]`. */
export function TripPage({ id }: Props) {
  const router = useRouter();
  // A deletion under way stops the polling: its `404` must not show the "trip not found" state.
  const [deleting, setDeleting] = useState(false);
  const trip = useTripMonitor(id, router.replace, deleting);
  const { analysis, provisional, provisionalExpired, notice, updatedAt, driverName } = trip;
  const actions = useMemo(
    () => <DeleteTripAction tripId={id} driverName={driverName} onDeleting={setDeleting} />,
    [id, driverName],
  );
  // The same element between polls that bring nothing new: the analysis is not rendered again.
  const view = useMemo(
    () =>
      analysis ? (
        <AnalysisView
          analysis={analysis}
          provisional={provisional}
          provisionalExpired={provisionalExpired}
          notice={notice}
          updatedAt={updatedAt}
          actions={actions}
        />
      ) : null,
    [analysis, provisional, provisionalExpired, notice, updatedAt, actions],
  );

  switch (trip.phase) {
    case "loading":
      return (
        <SimplePage text={trip.notice}>
          <LoadingState />
        </SimplePage>
      );
    case "waiting":
      return (
        <SimplePage text={trip.notice} actions={actions}>
          <WaitingState driverName={trip.driverName} />
        </SimplePage>
      );
    case "limit":
      return (
        <SimplePage actions={actions}>
          <WaitLimitState onContinue={trip.keepWaiting} />
        </SimplePage>
      );
    case "failed":
      return (
        <SimplePage actions={actions}>
          <FailedState />
        </SimplePage>
      );
    case "not_found":
      return (
        <SimplePage>
          <NotFoundState />
        </SimplePage>
      );
    case "error":
      return (
        <SimplePage>
          <ErrorState onRetry={trip.retry} />
        </SimplePage>
      );
    case "ready":
      return view;
  }
}
