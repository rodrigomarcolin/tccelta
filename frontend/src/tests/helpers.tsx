import { render } from "@testing-library/react";
import type { ReactElement } from "react";
import type { TripAnalysis } from "@/domain/trip/types";
import type { Theme } from "@/config";
import { ThemeProvider } from "@/components/ui/Theme";
import { buildAnalysis } from "@/server/trips/mock/analyses";
import type { ScenarioId } from "@/server/trips/mock/scenarios";

export function renderWithTheme(ui: ReactElement, initial: Theme = "dark") {
  return render(<ThemeProvider initial={initial}>{ui}</ThemeProvider>);
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
