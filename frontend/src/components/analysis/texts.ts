import type { TripWarning } from "@/domain/trip/types";

const TEXTS: Record<string, string> = {
  sem_gps: "Sem posições de GPS nesta viagem: a rota não pode ser mostrada",
  sem_hora_absoluta: "Esta viagem não tem hora absoluta. O eixo mostra o tempo desde o início",
  partes_sobrepostas:
    "Duas partes se sobrepõem no tempo; a anterior foi cortada no início da seguinte",
  sem_leituras: "Sem dados para esta viagem",
  motor_nunca_ligado: "O motor não ligou nesta viagem. Os valores usam todos os dados",
  cobertura_maf_baixa: "O consumo foi estimado com poucas leituras do sensor de ar",
  consumo_pode_estar_subestimado:
    "Consumo pode estar subestimado: houve muito tempo com pedal alto",
  marcha_lenta_reserva: "A rotação de marcha lenta usada é um valor de reserva",
  dados_parciais: "Calculada com os dados que chegaram em 20 s. Pode estar incompleta",
};

/** Text for a warning; an unknown code does not break the screen. */
export function warningText(warning: TripWarning): string {
  if (warning.codigo === "boot_sem_hora") {
    const detail = warning.detalhe ? ` (${warning.detalhe})` : "";
    return `Um boot ficou de fora por não ter hora absoluta${detail}`;
  }
  return TEXTS[warning.codigo] ?? `Aviso: ${warning.codigo}`;
}
