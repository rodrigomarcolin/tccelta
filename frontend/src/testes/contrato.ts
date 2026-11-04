import type { Analise } from "@/contrato/tipos";

/**
 * Valida uma análise contra o contrato da spec 02: arrays paralelos de mesmo
 * tamanho, `null` (e não zero) onde falta dado, separadores entre partes e
 * faixas em movimento somando 100%. Devolve a lista de violações.
 */
export function violacoesDoContrato(a: Analise): string[] {
  const erros: string[] = [];
  const n = a.series.t_ms.length;

  for (const [chave, valor] of Object.entries(a.series)) {
    if (Array.isArray(valor) && valor.length !== n) {
      erros.push(`series.${chave} tem ${valor.length} itens, esperado ${n}`);
    }
  }

  const lentas = a.series_lentas;
  const comp = (nome: string, arrs: unknown[][]) => {
    const t = arrs[0].length;
    if (arrs.some((x) => x.length !== t)) erros.push(`${nome}: arrays de tamanhos diferentes`);
  };
  comp("temperatura", [lentas.temperatura.t_ms, lentas.temperatura.c]);
  comp("nivel_tanque", [
    lentas.nivel_tanque.t_ms,
    lentas.nivel_tanque.pct,
    lentas.nivel_tanque.mediana_movel_pct,
  ]);

  if (a.rota) {
    comp("rota", [a.rota.t_ms, a.rota.lat, a.rota.lon]);
    const seg = a.rota.segmentos;
    if (seg.velocidade_kmh.length !== seg.faixa.length) erros.push("rota.segmentos desiguais");
    if (seg.faixa.length !== Math.max(0, a.rota.t_ms.length - 1)) {
      erros.push("rota.segmentos deve ter um item a menos que os pontos");
    }
    seg.faixa.forEach((f, i) => {
      if ((f === "falha_gps" || f === "sem_velocidade") && seg.velocidade_kmh[i] !== null) {
        erros.push(`segmento ${i} sem dado deve ter velocidade null`);
      }
    });
  }

  // Separadores entre partes: uma linha de valores null por pausa.
  const partes = a.viagem.partes.length;
  if (n > 0) {
    const separadores = a.series.t_ms.filter(
      (_, i) => a.series.rpm[i] === null && a.series.velocidade_kmh[i] === null,
    );
    if (partes > 1 && separadores.length !== partes - 1) {
      erros.push(`esperado ${partes - 1} separadores, achou ${separadores.length}`);
    }
  }
  for (const v of a.series.velocidade_kmh) {
    if (v !== null && v < 0) erros.push("velocidade negativa");
  }

  const emMov = a.faixas_velocidade.filter((f) => f.id !== "parado");
  if (emMov.length) {
    const soma = emMov.reduce((s, f) => s + (f.distancia_pct ?? 0), 0);
    if (Math.abs(soma - 100) > 0.5) erros.push(`faixas em movimento somam ${soma}`);
  }
  const parado = a.faixas_velocidade.find((f) => f.id === "parado");
  if (parado && parado.distancia_pct !== null) erros.push("parado deve ter distancia_pct null");

  if (a.viagem.relogio === null && (a.viagem.inicio_em !== null || a.viagem.fim_em !== null)) {
    erros.push("sem relógio, inicio_em e fim_em devem ser null");
  }
  if (a.rota === null) {
    for (const x of [...a.paradas, ...a.eventos]) {
      if (x.lat !== null || x.lon !== null) erros.push("sem rota, posições devem ser null");
    }
  }
  if (a.marchas.patamares.length === 0 && a.marchas.trocas.length > 0) {
    erros.push("trocas sem patamares");
  }
  for (const f of a.distribuicoes.rotacao) {
    if (f.pct < 0 || f.pct > 100) erros.push("pct fora de 0..100");
  }
  return erros;
}
