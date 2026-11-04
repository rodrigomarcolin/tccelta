"use client";

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import {
  TIMEZONE,
  WAIT_LIMIT_MS,
  POLL_WAITING_MS,
  POLL_FAILURES_NOTICE,
  POLL_PROVISIONAL_MAX_MS,
  POLL_PROVISIONAL_MS,
  POLL_BACKOFF_MAX_MS,
} from "@/config";
import { isProblem, type TripAnalysis, type TripStatusResponse } from "@/domain/trip/types";
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

type Phase = "loading" | "waiting" | "limit" | "failed" | "not_found" | "error" | "ready";

type State = {
  phase: Phase;
  driverName: string | null;
  analysis: TripAnalysis | null;
  provisional: boolean;
  provisionalExpired: boolean;
  /** Connection or busy-server notice, without clearing the screen. */
  notice: string | null;
  /** HH:MM of the last analysis version change. */
  updatedAt: string | null;
};

const INITIAL: State = {
  phase: "loading",
  driverName: null,
  analysis: null,
  provisional: false,
  provisionalExpired: false,
  notice: null,
  updatedAt: null,
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

type Read = { status: number; body: unknown; retryAfter?: number };

/** Waiting, polling and display of the analysis for `/viagem/[id]`. */
export function TripPage({ id, goTo = defaultGoTo }: Props) {
  const [state, setState] = useState<State>(INITIAL);
  const [attempt, setAttempt] = useState(0);
  const goToRef = useRef(goTo);
  useEffect(() => {
    goToRef.current = goTo;
  });

  useEffect(() => {
    let active = true;
    let stopped = false;
    let inFlight = false;
    /** A cycle was deferred because the tab is hidden. */
    let paused = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let failures = 0;
    let provisionalStart: number | null = null;
    let hasAnalysis = false;
    let currentCalculated: string | null = null;
    const waitDeadline = Date.now() + WAIT_LIMIT_MS;
    const controller = new AbortController();

    const update = (partial: Partial<State>) => setState((s) => ({ ...s, ...partial }));
    const clear = () => {
      if (timer !== undefined) clearTimeout(timer);
      timer = undefined;
    };
    const stop = () => {
      stopped = true;
      clear();
    };
    const schedule = (ms: number) => {
      clear();
      paused = false;
      if (!active || stopped) return;
      if (document.hidden) {
        paused = true;
        return;
      }
      timer = setTimeout(() => void cycle(), ms);
    };

    const sessionExpired = () => {
      stop();
      goToRef.current(`/entrar?retorno=${encodeURIComponent(`/viagem/${id}`)}`);
    };

    /** Network failure or 5xx: counts, backs off (1, 2, 4, up to 5 s) and warns on the third in a row. */
    const failure = () => {
      failures += 1;
      if (failures >= POLL_FAILURES_NOTICE) update({ notice: NETWORK_NOTICE });
      schedule(Math.min(POLL_BACKOFF_MAX_MS, POLL_WAITING_MS * 2 ** (failures - 1)));
    };

    const busy = (retryAfter: number | undefined) => {
      const s = Math.max(1, retryAfter ?? 1);
      update({ notice: `Servidor ocupado. Tentando de novo em ${s} s.` });
      schedule(s * 1000);
    };

    const unexpectedError = () => {
      if (hasAnalysis) {
        failure();
        return;
      }
      stop();
      update({ phase: "error", notice: null });
    };

    const read = async (url: string): Promise<Read> => {
      const r = await fetch(url, { cache: "no-store", signal: controller.signal });
      const body: unknown = await r.json().catch(() => null);
      const ra = Number(r.headers?.get("Retry-After"));
      return {
        status: r.status,
        body,
        retryAfter: Number.isFinite(ra) && ra > 0 ? ra : undefined,
      };
    };

    /** Handles what both routes share. Returns `true` if it already decided what to do. */
    const commonFailure = (r: Read): boolean => {
      // System credential refused or access blocked: the server already held
      // the attempts until the deadline; polling pauses for that time and does not insist.
      const code = isProblem(r.body) ? r.body.codigo : undefined;
      if (code === "credencial_invalida" || code === "bloqueado") {
        const s = Math.max(1, r.retryAfter ?? 60);
        update({ notice: code === "bloqueado" ? blockedNotice(s) : CREDENTIAL_NOTICE });
        schedule(s * 1000);
        return true;
      }
      if (r.status === 401) {
        sessionExpired();
        return true;
      }
      if (r.status === 429) {
        busy(r.retryAfter);
        return true;
      }
      if (r.status >= 500) {
        failure();
        return true;
      }
      return false;
    };

    async function step() {
      const st = await read(`/api/trips/${id}`);
      if (!active || commonFailure(st)) return;
      if (st.status === 404) {
        stop();
        setState({ ...INITIAL, phase: "not_found" });
        return;
      }
      const body = st.body as TripStatusResponse | null;
      if (st.status !== 200 || !body || typeof body.status !== "string") {
        unexpectedError();
        return;
      }
      failures = 0;
      const driverName = body.motorista.nome;

      if (body.status === "pendente" || body.status === "aguardando_dados") {
        if (Date.now() >= waitDeadline) {
          stop();
          update({ phase: "limit", driverName, notice: null });
          return;
        }
        update({ phase: "waiting", driverName, notice: null });
        schedule(POLL_WAITING_MS);
        return;
      }
      if (body.status === "falhou") {
        stop();
        setState({ ...INITIAL, phase: "failed", driverName });
        return;
      }

      // Ready: fetches the analysis if there is none yet or if `calculada_em` changed.
      const calculated = body.calculada_em;
      const changed = hasAnalysis && calculated !== currentCalculated;
      let updatedAt: string | undefined;
      if (!hasAnalysis || changed) {
        const a = await read(`/api/trips/${id}/analysis`);
        if (!active || commonFailure(a)) return;
        if (a.status === 409) {
          schedule(POLL_WAITING_MS);
          return;
        }
        if (a.status !== 200 || !a.body) {
          unexpectedError();
          return;
        }
        if (changed && calculated) updatedAt = formatClock(calculated) ?? undefined;
        hasAnalysis = true;
        currentCalculated = calculated;
        update({ analysis: a.body as TripAnalysis, phase: "ready" });
      }
      failures = 0;
      setState((s) => ({
        ...s,
        driverName,
        provisional: body.provisoria,
        notice: null,
        updatedAt: updatedAt ?? s.updatedAt,
      }));

      if (!body.provisoria) {
        stop();
        return;
      }
      provisionalStart ??= Date.now();
      if (Date.now() - provisionalStart < POLL_PROVISIONAL_MAX_MS) {
        schedule(POLL_PROVISIONAL_MS);
        return;
      }
      stop();
      update({ provisionalExpired: true });
    }

    async function cycle() {
      if (!active || stopped || inFlight) return;
      inFlight = true;
      try {
        await step();
      } catch {
        if (active) failure();
      } finally {
        inFlight = false;
      }
    }

    const onVisibilityChange = () => {
      if (stopped) return;
      if (document.hidden) {
        if (timer !== undefined) {
          clear();
          paused = true;
        }
      } else if (paused) {
        paused = false;
        void cycle();
      }
    };
    const onBackOnline = () => {
      if (stopped || inFlight) return;
      clear();
      paused = false;
      void cycle();
    };
    document.addEventListener("visibilitychange", onVisibilityChange);
    window.addEventListener("online", onBackOnline);

    void cycle();
    return () => {
      active = false;
      controller.abort();
      clear();
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.removeEventListener("online", onBackOnline);
    };
  }, [id, attempt]);

  const tryAgain = useCallback(() => {
    setState(INITIAL);
    setAttempt((n) => n + 1);
  }, []);

  const keepWaiting = useCallback(() => {
    setState((s) => ({ ...s, phase: "waiting" }));
    setAttempt((n) => n + 1);
  }, []);

  switch (state.phase) {
    case "loading":
      return (
        <SimplePage text={state.notice}>
          <LoadingState />
        </SimplePage>
      );
    case "waiting":
      return (
        <SimplePage text={state.notice}>
          <WaitingState driverName={state.driverName} />
        </SimplePage>
      );
    case "limit":
      return (
        <SimplePage>
          <WaitLimitState onContinue={keepWaiting} />
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
          <ErrorState onRetry={tryAgain} />
        </SimplePage>
      );
    case "ready":
      return state.analysis ? (
        <AnalysisView
          analysis={state.analysis}
          provisional={state.provisional}
          provisionalExpired={state.provisionalExpired}
          notice={state.notice}
          updatedAt={state.updatedAt}
        />
      ) : null;
  }
}
