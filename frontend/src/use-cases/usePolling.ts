import { useEffect, useRef } from "react";
import type { FetchStatus } from "@tanstack/react-query";

type Poll = {
  fetchStatus: FetchStatus;
  failureCount: number;
  dataUpdatedAt: number;
  errorUpdatedAt: number;
  /** Starts a fetch, unless one is in flight. */
  refetch: () => void;
  /** Drops the fetch in flight (and its failure streak) and starts another. */
  restart: () => void;
};

/**
 * Next poll `delay` ms after each fetch ends (`false` stops polling). React Query's own
 * `refetchInterval` is not used: it ticks on a fixed clock, so a slow response would be
 * followed too soon, and the ticks of a hidden tab are lost instead of deferred.
 *
 * - Hidden tab: the pending poll is deferred, and runs right away on return. Without a
 *   deferred poll, returning does nothing.
 * - `online`: forces an attempt. React Query does not fire it when it already thinks it is online.
 * - Nothing happens once polling has stopped.
 */
export function usePolling(poll: Poll, delay: number | false) {
  const { fetchStatus, dataUpdatedAt, errorUpdatedAt } = poll;
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const deferred = useRef(false);
  const latest = useRef({ ...poll, delay });
  useEffect(() => {
    latest.current = { ...poll, delay };
  });

  useEffect(() => {
    deferred.current = false;
    if (fetchStatus !== "idle" || delay === false) return;
    if (document.hidden) {
      deferred.current = true;
      return;
    }
    // Counted from when the response arrived, not from when the screen caught up with it.
    const sinceSettled = Date.now() - Math.max(dataUpdatedAt, errorUpdatedAt);
    const id = setTimeout(
      () => {
        timer.current = undefined;
        latest.current.refetch();
      },
      Math.max(0, delay - sinceSettled),
    );
    timer.current = id;
    return () => {
      clearTimeout(id);
      timer.current = undefined;
    };
  }, [fetchStatus, delay, dataUpdatedAt, errorUpdatedAt]);

  useEffect(() => {
    const clear = () => {
      if (timer.current !== undefined) clearTimeout(timer.current);
      timer.current = undefined;
    };
    const onVisibilityChange = () => {
      if (latest.current.delay === false) return;
      if (document.hidden) {
        if (timer.current !== undefined) {
          clear();
          deferred.current = true;
        }
      } else if (deferred.current) {
        deferred.current = false;
        latest.current.refetch();
      }
    };
    const onOnline = () => {
      const {
        delay: current,
        fetchStatus: status,
        failureCount,
        refetch,
        restart,
      } = latest.current;
      if (current === false) return;
      if (status === "idle") {
        clear();
        deferred.current = false;
        refetch();
      } else if (failureCount > 0) {
        restart();
      }
    };
    document.addEventListener("visibilitychange", onVisibilityChange);
    window.addEventListener("online", onOnline);
    return () => {
      document.removeEventListener("visibilitychange", onVisibilityChange);
      window.removeEventListener("online", onOnline);
    };
  }, []);
}
