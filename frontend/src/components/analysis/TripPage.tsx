"use client";

import { useMemo, type ReactNode } from "react";
import { useRouter } from "next/navigation";
import { useTripMonitor } from "@/use-cases/useTripMonitor";
import { HeaderControls } from "@/components/ui/AppNav";
import { PageHeader } from "@/components/ui/PageHeader";
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

function SimplePage({ children, text }: { children: ReactNode; text?: string | null }) {
  return (
    <div>
      <PageHeader>
        <h1>Análise de viagem</h1>
        <HeaderControls />
      </PageHeader>
      {text ? <ConnectionBanner text={text} /> : null}
      {children}
    </div>
  );
}

/** Waiting, polling and display of the analysis for `/viagem/[id]`. */
export function TripPage({ id }: Props) {
  const router = useRouter();
  const trip = useTripMonitor(id, router.replace);
  const { analysis, provisional, provisionalExpired, notice, updatedAt } = trip;
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
        />
      ) : null,
    [analysis, provisional, provisionalExpired, notice, updatedAt],
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
        <SimplePage text={trip.notice}>
          <WaitingState driverName={trip.driverName} />
        </SimplePage>
      );
    case "limit":
      return (
        <SimplePage>
          <WaitLimitState onContinue={trip.keepWaiting} />
        </SimplePage>
      );
    case "failed":
      return (
        <SimplePage>
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
