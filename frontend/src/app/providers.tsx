"use client";

import { useState, type ReactNode } from "react";
import { notifyManager, QueryClient, QueryClientProvider } from "@tanstack/react-query";

/**
 * Without `networkMode: "always"`, React Query pauses requests while `navigator.onLine` is
 * false instead of letting them fail, which hides the "no connection" notice.
 */
export function createQueryClient(): QueryClient {
  // Notifies on a microtask instead of the default `setTimeout(0)`: the screen follows the
  // response without waiting for a timer.
  notifyManager.setScheduler(queueMicrotask);
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        refetchOnWindowFocus: false,
        refetchOnReconnect: false,
        networkMode: "always",
      },
      mutations: { retry: false, networkMode: "always" },
    },
  });
}

export function Providers({ children }: { children: ReactNode }) {
  const [client] = useState(createQueryClient);
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}
