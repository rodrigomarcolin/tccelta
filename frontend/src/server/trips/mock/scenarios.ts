import type { TripInput } from "@/domain/trip/types";

export type ScenarioId =
  | "normal"
  | "sem_gps"
  | "gps_falhas"
  | "sem_hora"
  | "sem_leituras"
  | "provisoria_aguardando"
  | "pid_ausente"
  | "dados_parciais"
  | "intervalo";

export const BOOT_BY_SCENARIO: Partial<Record<number, ScenarioId>> = {
  8: "normal",
  9: "sem_gps",
  10: "gps_falhas",
  11: "sem_hora",
  12: "sem_leituras",
  13: "provisoria_aguardando",
  14: "pid_ausente",
  15: "dados_parciais",
};

export function scenarioForInput(input: TripInput): ScenarioId {
  if (input.tipo === "intervalo") return "intervalo";
  return BOOT_BY_SCENARIO[input.boot_id] ?? "normal";
}

/** Scenarios that stay provisional until `PROVISIONAL_MS` after creation. */
export const PROVISIONAL_SCENARIOS: ScenarioId[] = [
  "sem_leituras",
  "provisoria_aguardando",
  "dados_parciais",
];

export const PROVISIONAL_MS = 30_000;
export const PENDING_UNTIL_MS = 2_000;
export const AWAITING_UNTIL_MS = 5_000;

export type Example = { id: string; scenario: ScenarioId; label: string; input: TripInput };

function boot(id: number): TripInput {
  return { tipo: "boot", dongle_id: 1, boot_generation: 0, boot_id: id };
}

/** Fixed example trips, one per scenario, to open by URL. */
export const EXAMPLES: Example[] = [
  {
    id: "7d1f3a10-0001-4c5e-9b1a-000000000008",
    scenario: "normal",
    label: "Boot normal",
    input: boot(8),
  },
  {
    id: "7d1f3a10-0002-4c5e-9b1a-000000000009",
    scenario: "sem_gps",
    label: "Sem GPS",
    input: boot(9),
  },
  {
    id: "7d1f3a10-0003-4c5e-9b1a-000000000010",
    scenario: "gps_falhas",
    label: "GPS com falhas",
    input: boot(10),
  },
  {
    id: "7d1f3a10-0004-4c5e-9b1a-000000000011",
    scenario: "sem_hora",
    label: "Sem hora absoluta",
    input: boot(11),
  },
  {
    id: "7d1f3a10-0005-4c5e-9b1a-000000000012",
    scenario: "sem_leituras",
    label: "Sem leituras",
    input: boot(12),
  },
  {
    id: "7d1f3a10-0006-4c5e-9b1a-000000000013",
    scenario: "provisoria_aguardando",
    label: "Provisória aguardando dados",
    input: boot(13),
  },
  {
    id: "7d1f3a10-0007-4c5e-9b1a-000000000014",
    scenario: "pid_ausente",
    label: "PID ausente",
    input: boot(14),
  },
  {
    id: "7d1f3a10-0008-4c5e-9b1a-000000000015",
    scenario: "dados_parciais",
    label: "Dados parciais",
    input: boot(15),
  },
  {
    id: "7d1f3a10-0009-4c5e-9b1a-000000000099",
    scenario: "intervalo",
    label: "Intervalo com duas partes",
    input: {
      tipo: "intervalo",
      dongle_id: 1,
      t0: "2026-10-28T11:00:00.000Z",
      t1: "2026-10-28T21:00:00.000Z",
    },
  },
];
