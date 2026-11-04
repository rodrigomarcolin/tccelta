import type { PropsGrafico } from "@/componentes/analise/GraficoUPlot";
import type { PropsMapa } from "@/componentes/analise/MapaRotaLeaflet";

/**
 * Dublês de Grafico e Mapa para jsdom (sem canvas nem Leaflet).
 * Cada um expõe os dados recebidos e botões que simulam o mouse.
 */
export function GraficoDuble(p: PropsGrafico) {
  if (p.semLeituras) return <div data-testid="grafico-vazio">{p.titulo}: Sem leituras</div>;
  const meio = p.x[Math.floor(p.x.length / 2)];
  return (
    <div
      data-testid="grafico"
      data-titulo={p.titulo}
      data-cursor-x={p.cursorX ?? ""}
      data-aplicar={String(p.aplicarCursor)}
      data-marcadores={p.marcadores?.length ?? 0}
      data-pausas={p.pausas?.length ?? 0}
      data-referencia={p.referenciaY?.rotulo ?? ""}
      data-series={p.series.length}
    >
      <button type="button" onClick={() => p.onCursorX?.(meio)}>
        {`mover ${p.titulo}`}
      </button>
      <button type="button" onClick={() => p.onCursorX?.(null)}>
        {`sair ${p.titulo}`}
      </button>
    </div>
  );
}

export function MapaDuble(p: PropsMapa) {
  return (
    <div
      data-testid="mapa"
      data-cursor-t={p.cursorT ?? ""}
      data-foco={p.foco ? `${p.foco.lat},${p.foco.lon}` : ""}
      data-tema={p.tema}
      data-eventos={p.eventos.length}
    >
      <button type="button" onClick={() => p.onHoverT(p.rota.t_ms[10])}>
        hover na rota
      </button>
      <button type="button" onClick={() => p.onHoverT(null)}>
        sair da rota
      </button>
    </div>
  );
}
