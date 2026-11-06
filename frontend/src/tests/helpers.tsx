import { render } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import type { ReactElement, ReactNode } from "react";
import type { TripAnalysis } from "@/domain/trip/types";
import type { Theme } from "@/config";
import { ThemeProvider } from "@/components/ui/Theme";
import { createQueryClient } from "@/app/providers";
import { buildAnalysis } from "@/server/trips/mock/analyses";
import type { ScenarioId } from "@/server/trips/mock/scenarios";

export function renderWithTheme(ui: ReactElement, initial: Theme = "dark") {
  // A new client per test: no cache shared between them.
  const client = createQueryClient();
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        <ThemeProvider initial={initial}>{children}</ThemeProvider>
      </QueryClientProvider>
    );
  }
  return render(ui, { wrapper: Wrapper });
}

const cache = new Map<string, TripAnalysis>();

/** Analysis of a mock scenario, for the screen tests. */
export function analysisFor(scenario: ScenarioId, complete = false, boot = 8): TripAnalysis {
  const key = `${scenario}:${complete}:${boot}`;
  let a = cache.get(key);
  if (!a) {
    a = buildAnalysis(
      scenario,
      {
        input:
          scenario === "intervalo"
            ? {
                tipo: "intervalo",
                dongle_id: 1,
                t0: "2026-10-28T11:00:00.000Z",
                t1: "2026-10-28T21:00:00.000Z",
              }
            : { tipo: "boot", dongle_id: 1, boot_id: boot },
        driver: { id: "11111111-1111-4111-8111-111111111111", nome: "Maria" },
        category: "feira",
        fuel: "gasolina_e30",
      },
      complete,
    );
    cache.set(key, a);
  }
  // Deep copy: tests may mutate the object.
  return structuredClone(a);
}
