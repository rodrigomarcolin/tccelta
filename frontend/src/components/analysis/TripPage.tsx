"use client";

import { useMemo, type ReactNode } from "react";
import { useTripMonitor } from "@/use-cases/useTripMonitor";
import { ThemeToggle } from "@/components/ui/Theme";
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

type Props = {
  id: string;
  /** Replaces navigation (tests). */
  goTo?: (url: string) => void;
};

function defaultGoTo(url: string) {
  window.location.assign(url);
}

function SimplePage({ children, text }: { children: ReactNode; text?: string | null }) {
  return (
    <div className="analysis">
      <header className="header">
        <h1>Análise de viagem</h1>
        <ThemeToggle />
      </header>
      {text ? <ConnectionBanner text={text} /> : null}
      {children}
    </div>
  );
}

/** Waiting, polling and display of the analysis for `/viagem/[id]`. */
export function TripPage({ id, goTo = defaultGoTo }: Props) {
  const trip = useTripMonitor(id, goTo);
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
