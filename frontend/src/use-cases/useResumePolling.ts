import { useEffect, useRef } from "react";
import type { FetchStatus } from "@tanstack/react-query";

type Resume = {
  /** The status query `refetchInterval`: `false` means polling has stopped. */
  delay: number | false;
  fetchStatus: FetchStatus;
  failureCount: number;
  /** Starts a fetch, unless one is in flight. */
  refetch: () => void;
  /** Drops the fetch in flight (and its failure streak) and starts another. */
  restart: () => void;
};

/**
 * What `refetchInterval` does not do on its own. Neither runs once polling has stopped.
 *
 * - Returning to the tab: React Query skips the interval ticks of a hidden tab, so the next
 *   poll would wait for a full interval. If the tab was hidden while polling was active
 *   (also when the page mounted hidden), it polls right away on return.
 * - `online`: forces an attempt. React Query does not fire its own reconnect when it
 *   already thinks it is online.
 */
export function useResumePolling(resume: Resume) {
  const { delay } = resume;
  const latest = useRef(resume);
  useEffect(() => {
    latest.current = resume;
  });
  /** The tab was hidden while polling was active. */
  const missed = useRef(false);

  useEffect(() => {
    if (delay === false) missed.current = false;
    else if (document.hidden) missed.current = true;
  }, [delay]);

  useEffect(() => {
    const onVisibilityChange = () => {
      if (latest.current.delay === false) return;
      if (document.hidden) {
        missed.current = true;
      } else if (missed.current) {
        missed.current = false;
        latest.current.refetch();
      }
    };
    const onOnline = () => {
      const { delay: current, fetchStatus, failureCount, refetch, restart } = latest.current;
      if (current === false) return;
      if (fetchStatus === "idle") refetch();
      else if (failureCount > 0) restart();
    };
    document.addEventListener("visibilitychange", onVisibilityChange);
    window.addEventListener("online", onOnline);
    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.removeEventListener("online", onOnline);
    };
  }, []);
}
