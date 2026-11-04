import type { Entrada } from "@/contrato/tipos";

export type CenarioId =
  | "normal"
  | "sem_gps"
  | "gps_falhas"
  | "sem_hora"
  | "sem_leituras"
  | "provisoria_aguardando"
  | "pid_ausente"
  | "dados_parciais"
  | "intervalo";

export const BOOT_POR_CENARIO: Partial<Record<number, CenarioId>> = {
  8: "normal",
  9: "sem_gps",
  10: "gps_falhas",
  11: "sem_hora",
  12: "sem_leituras",
  13: "provisoria_aguardando",
  14: "pid_ausente",
  15: "dados_parciais",
};

export function cenarioDaEntrada(entrada: Entrada): CenarioId {
  if (entrada.tipo === "intervalo") return "intervalo";
  return BOOT_POR_CENARIO[entrada.boot_id] ?? "normal";
}

/** Cenários que ficam provisórios até `PROVISORIA_MS` depois da criação. */
export const CENARIOS_PROVISORIOS: CenarioId[] = [
  "sem_leituras",
  "provisoria_aguardando",
  "dados_parciais",
];

export const PROVISORIA_MS = 30_000;
export const PENDENTE_ATE_MS = 2_000;
export const AGUARDANDO_ATE_MS = 5_000;

export type Exemplo = { id: string; cenario: CenarioId; rotulo: string; entrada: Entrada };

function boot(id: number): Entrada {
  return { tipo: "boot", dongle_id: 1, boot_generation: 0, boot_id: id };
}

/** Viagens de exemplo fixas, uma por cenário, para abrir por URL. */
export const EXEMPLOS: Exemplo[] = [
  {
    id: "7d1f3a10-0001-4c5e-9b1a-000000000008",
    cenario: "normal",
    rotulo: "Boot normal",
    entrada: boot(8),
  },
  {
    id: "7d1f3a10-0002-4c5e-9b1a-000000000009",
    cenario: "sem_gps",
    rotulo: "Sem GPS",
    entrada: boot(9),
  },
  {
    id: "7d1f3a10-0003-4c5e-9b1a-000000000010",
    cenario: "gps_falhas",
    rotulo: "GPS com falhas",
    entrada: boot(10),
  },
  {
    id: "7d1f3a10-0004-4c5e-9b1a-000000000011",
    cenario: "sem_hora",
    rotulo: "Sem hora absoluta",
    entrada: boot(11),
  },
  {
    id: "7d1f3a10-0005-4c5e-9b1a-000000000012",
    cenario: "sem_leituras",
    rotulo: "Sem leituras",
    entrada: boot(12),
  },
  {
    id: "7d1f3a10-0006-4c5e-9b1a-000000000013",
    cenario: "provisoria_aguardando",
    rotulo: "Provisória aguardando dados",
    entrada: boot(13),
  },
  {
    id: "7d1f3a10-0007-4c5e-9b1a-000000000014",
    cenario: "pid_ausente",
    rotulo: "PID ausente",
    entrada: boot(14),
  },
  {
    id: "7d1f3a10-0008-4c5e-9b1a-000000000015",
    cenario: "dados_parciais",
    rotulo: "Dados parciais",
    entrada: boot(15),
  },
  {
    id: "7d1f3a10-0009-4c5e-9b1a-000000000099",
    cenario: "intervalo",
    rotulo: "Intervalo com duas partes",
    entrada: {
      tipo: "intervalo",
      dongle_id: 1,
      t0: "2026-10-28T11:00:00.000Z",
      t1: "2026-10-28T21:00:00.000Z",
    },
  },
];
