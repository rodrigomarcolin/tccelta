import { useCallback, useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import {
  POLL_FAILURES_NOTICE,
  POLL_PROVISIONAL_MAX_MS,
  POLL_PROVISIONAL_MS,
  POLL_WAITING_MS,
  TIMEZONE,
  WAIT_LIMIT_MS,
} from "@/config";
import { ApiError } from "@/domain/trip/apiError";
import type { TripAnalysis } from "@/domain/trip/types";
import { apiCode } from "./retryPolicy";
import { usePolling } from "./usePolling";
import { tripStatusKey, useTripStatus } from "./useTripStatus";
import { useTripAnalysis } from "./useTripAnalysis";

export type Phase = "loading" | "waiting" | "limit" | "failed" | "not_found" | "error" | "ready";

export type TripMonitor = {
  phase: Phase;
  driverName: string | null;
  analysis: TripAnalysis | null;
  provisional: boolean;
  provisionalExpired: boolean;
  /** Connection or busy-server notice, without clearing the screen. */
  notice: string | null;
  /** HH:MM of the last analysis version change. */
  updatedAt: string | null;
  /** Starts over from the initial state. */
  retry: () => void;
  /** Gives the wait another full limit. */
  keepWaiting: () => void;
};

const NETWORK_NOTICE = "Sem conexão com o servidor. Tentando novamente…";
const CREDENTIAL_NOTICE = "O servidor não aceitou a credencial do sistema. Chame a organização.";
const blockedNotice = (s: number) =>
  `Acesso temporariamente bloqueado. Tente de novo em ${Math.ceil(s)} s.`;

const formatClock = (iso: string): string | null => {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  return new Intl.DateTimeFormat("pt-BR", {
    timeZone: TIMEZONE,
    hour: "2-digit",
    minute: "2-digit",
  }).format(d);
};

/** Notice of the failure streak in progress, from what React Query counts within the fetch. */
function noticeOf(q: {
  fetchStatus: string;
  failureCount: number;
  failureReason: unknown;
}): string | null {
  if (q.fetchStatus === "idle" || q.failureCount === 0) return null;
  const e = q.failureReason;
  if (e instanceof ApiError) {
    const code = apiCode(e);
    if (code === "bloqueado") return blockedNotice(Math.max(1, e.retryAfter ?? 60));
    if (code === "credencial_invalida") return CREDENTIAL_NOTICE;
    if (e.status === 429) {
      return `Servidor ocupado. Tentando de novo em ${Math.max(1, e.retryAfter ?? 1)} s.`;
    }
    if (e.status === 409) return null;
  }
  return q.failureCount >= POLL_FAILURES_NOTICE ? NETWORK_NOTICE : null;
}

const isStatus = (e: unknown, status: number) => e instanceof ApiError && e.status === status;

type Session = {
  id: string;
  /** Limit of the wait in pendente or aguardando_dados. */
  deadline: number;
  /** When the provisional result was first shown. */
  provisionalSince: number | null;
  /** `calculada_em` of the analysis on screen (`undefined`: none yet). */
  loadedCalculatedAt: string | null | undefined;
  updatedAt: string | null;
  /** The waiting screen was shown, so a ready trip without analysis yet keeps it. */
  sawWaiting: boolean;
};

const fresh = (id: string): Session => ({
  id,
  deadline: Date.now() + WAIT_LIMIT_MS,
  provisionalSince: null,
  loadedCalculatedAt: undefined,
  updatedAt: null,
  sawWaiting: false,
});

/** Waiting, polling and display of the analysis of a trip, for `TripPage`. */
export function useTripMonitor(id: string, goTo: (url: string) => void): TripMonitor {
  const queryClient = useQueryClient();
  const hasAnalysis = useRef(false);
  const [session, setSession] = useState<Session>(() => fresh(id));

  const statusQuery = useTripStatus(id, hasAnalysis);
  const status = statusQuery.data;
  const ready = !statusQuery.isError && status?.status === "pronta";
  const analysisQuery = useTripAnalysis(id, status?.calculada_em ?? null, ready, hasAnalysis);
  const analysis = analysisQuery.data;

  useEffect(() => {
    hasAnalysis.current = analysis !== undefined;
  }, [analysis]);

  // Adjusts the session while rendering, from what just arrived.
  let current = session;
  if (current.id !== id) current = fresh(id);
  if (ready && analysis !== undefined && !analysisQuery.isPlaceholderData) {
    const calculatedAt = status?.calculada_em ?? null;
    if (current.loadedCalculatedAt !== calculatedAt) {
      const first = current.loadedCalculatedAt === undefined;
      current = {
        ...current,
        loadedCalculatedAt: calculatedAt,
        updatedAt:
          !first && calculatedAt
            ? (formatClock(calculatedAt) ?? current.updatedAt)
            : current.updatedAt,
      };
    }
  }
  if (ready && analysis !== undefined && status?.provisoria && current.provisionalSince === null) {
    current = { ...current, provisionalSince: statusQuery.dataUpdatedAt };
  }
  if (status?.status === "pendente" || status?.status === "aguardando_dados") {
    if (!current.sawWaiting) current = { ...current, sawWaiting: true };
  }
  if (current !== session) setSession(current);

  const waiting = status?.status === "pendente" || status?.status === "aguardando_dados";
  const waitLimit =
    !statusQuery.isError && waiting && statusQuery.dataUpdatedAt >= current.deadline;
  const provisional = ready && status?.provisoria === true;
  const provisionalExpired =
    provisional &&
    current.provisionalSince !== null &&
    statusQuery.dataUpdatedAt - current.provisionalSince >= POLL_PROVISIONAL_MAX_MS;

  const statusFailed = statusQuery.isError;
  const analysisFailed = analysisQuery.isError && analysis === undefined;

  let delay: number | false;
  if (statusFailed || analysisFailed || waitLimit) delay = false;
  else if (!status || waiting) delay = POLL_WAITING_MS;
  else if (status.status === "falhou") delay = false;
  else if (analysis === undefined) delay = POLL_WAITING_MS;
  else delay = provisional && !provisionalExpired ? POLL_PROVISIONAL_MS : false;

  const refetchStatus = statusQuery.refetch;
  const refetch = useCallback(() => void refetchStatus({ cancelRefetch: false }), [refetchStatus]);
  const restart = useCallback(() => {
    void queryClient.cancelQueries({ queryKey: tripStatusKey(id), exact: true });
    void refetchStatus();
  }, [queryClient, id, refetchStatus]);
  usePolling(
    {
      fetchStatus: statusQuery.fetchStatus,
      failureCount: statusQuery.failureCount,
      dataUpdatedAt: statusQuery.dataUpdatedAt,
      errorUpdatedAt: statusQuery.errorUpdatedAt,
      refetch,
      restart,
    },
    delay,
  );

  const sessionExpired = isStatus(statusQuery.error, 401) || isStatus(analysisQuery.error, 401);
  const goToRef = useRef(goTo);
  useEffect(() => {
    goToRef.current = goTo;
  });
  useEffect(() => {
    if (sessionExpired) goToRef.current(`/entrar?retorno=${encodeURIComponent(`/viagem/${id}`)}`);
  }, [sessionExpired, id]);

  let phase: Phase;
  if (statusFailed) {
    phase = isStatus(statusQuery.error, 404) ? "not_found" : sessionExpired ? "loading" : "error";
  } else if (!status) phase = "loading";
  else if (waiting) phase = waitLimit ? "limit" : "waiting";
  else if (status.status === "falhou") phase = "failed";
  else if (analysis !== undefined) phase = "ready";
  else if (analysisFailed) phase = sessionExpired ? "loading" : "error";
  else phase = current.sawWaiting ? "waiting" : "loading";

  const showsNotice = phase === "loading" || phase === "waiting" || phase === "ready";
  const notice = showsNotice ? (noticeOf(statusQuery) ?? noticeOf(analysisQuery)) : null;

  const retry = useCallback(() => {
    setSession(fresh(id));
    hasAnalysis.current = false;
    void queryClient.resetQueries({ queryKey: ["trip", id] });
  }, [queryClient, id]);

  const keepWaiting = useCallback(() => {
    setSession((s) => ({ ...s, deadline: Date.now() + WAIT_LIMIT_MS }));
    void refetchStatus();
  }, [refetchStatus]);

  return {
    phase,
    driverName: status?.motorista.nome ?? null,
    analysis: phase === "ready" ? (analysis ?? null) : null,
    provisional,
    provisionalExpired,
    notice,
    updatedAt: current.updatedAt,
    retry,
    keepWaiting,
  };
}
