"use client";

import { useEffect, useRef, useState } from "react";
import type * as Leaflet from "leaflet";
import "leaflet/dist/leaflet.css";
import { ATRIBUICAO_TILES, TILES, type Tema } from "@/config";
import { pontoDaRotaNoLugar, pontoDaRotaNoTempo } from "@/dominio/busca";
import { ROTULO_EVENTO } from "@/dominio/analise";
import type { Evento, Parada, Rota } from "@/contrato/tipos";
import { duracao } from "@/formato/numeros";
import { corCss } from "./cores";

export type PropsMapa = {
  rota: Rota;
  eventos: Evento[];
  paradas: Parada[];
  tema: Tema;
  cursorT: number | null;
  onHoverT: (t: number | null) => void;
  foco: { lat: number; lon: number; nonce: number } | null;
  formatarT: (t: number) => string;
};

const LIMITE_ERROS_TILE = 3;

function iconeEvento(L: typeof Leaflet, tipo: Evento["tipo"]): Leaflet.DivIcon | null {
  if (tipo === "marcha_lenta_longa") return null;
  const contorno = corCss("--text-primary");
  const preenche = corCss("--surface-card");
  const pontos = tipo === "frenagem_brusca" ? "2,3 16,3 9,16" : "2,15 16,15 9,2";
  return L.divIcon({
    className: "marcador-evento",
    html: `<svg width="18" height="18" viewBox="0 0 18 18" aria-hidden="true"><polygon points="${pontos}" fill="${preenche}" stroke="${contorno}" stroke-width="1.8" stroke-linejoin="round"/></svg>`,
    iconSize: [18, 18],
    iconAnchor: [9, 9],
  });
}

function iconeAB(L: typeof Leaflet, letra: "A" | "B"): Leaflet.DivIcon {
  return L.divIcon({
    className: "marcador-ab-wrap",
    html: `<span class="marcador-ab">${letra}</span>`,
    iconSize: [24, 24],
    iconAnchor: [12, 12],
  });
}

/** Mapa Leaflet com a rota colorida por segmento. Só roda no cliente. */
export default function MapaRotaLeaflet(props: PropsMapa) {
  const { rota, eventos, paradas, tema, cursorT, foco } = props;
  const el = useRef<HTMLDivElement>(null);
  const L = useRef<typeof Leaflet | null>(null);
  const mapa = useRef<Leaflet.Map | null>(null);
  const camada = useRef<Leaflet.LayerGroup | null>(null);
  const cursor = useRef<Leaflet.CircleMarker | null>(null);
  const tiles = useRef<{ claro?: Leaflet.TileLayer; escuro?: Leaflet.TileLayer }>({});
  const cbHover = useRef(props.onHoverT);
  useEffect(() => {
    cbHover.current = props.onHoverT;
  });
  const rotaRef = useRef(rota);
  useEffect(() => {
    rotaRef.current = rota;
  });
  const [pronto, setPronto] = useState(false);
  const [semBase, setSemBase] = useState(false);
  const [impressao, setImpressao] = useState(false);

  // Criação do mapa.
  useEffect(() => {
    let cancelado = false;
    let limpar: (() => void) | undefined;
    void import("leaflet").then((mod) => {
      const lib = (mod as unknown as { default?: typeof Leaflet }).default ?? mod;
      if (cancelado || !el.current) return;
      L.current = lib;
      const m = lib.map(el.current, { preferCanvas: true, zoomControl: true });
      mapa.current = m;
      camada.current = lib.layerGroup().addTo(m);

      let carregados = 0;
      let erros = 0;
      const opcoes = { attribution: ATRIBUICAO_TILES, maxZoom: 19 };
      const claro = lib.tileLayer(TILES.claro, { ...opcoes, className: "tile-claro" });
      const escuro = lib.tileLayer(TILES.escuro, { ...opcoes, className: "tile-escuro" });
      for (const t of [claro, escuro]) {
        t.on("tileload", () => (carregados += 1));
        t.on("tileerror", () => {
          erros += 1;
          if (carregados === 0 && erros >= LIMITE_ERROS_TILE) {
            claro.remove();
            escuro.remove();
            setSemBase(true);
          }
        });
      }
      tiles.current = { claro, escuro };
      claro.addTo(m);
      escuro.addTo(m);

      const pts = rotaRef.current.lat
        .map((la, i) => [la, rotaRef.current.lon[i]] as [number | null, number | null])
        .filter((p): p is [number, number] => p[0] !== null && p[1] !== null);
      if (pts.length) m.fitBounds(lib.latLngBounds(pts), { padding: [24, 24] });
      else m.setView([-25.43, -49.27], 13);

      m.on("mousemove", (ev: Leaflet.LeafletMouseEvent) => {
        const mPorPx = (156543.03 * Math.cos((ev.latlng.lat * Math.PI) / 180)) / 2 ** m.getZoom();
        const ponto = pontoDaRotaNoLugar(
          rotaRef.current,
          ev.latlng.lat,
          ev.latlng.lng,
          mPorPx * 16,
        );
        cbHover.current(ponto ? ponto.t : null);
      });
      m.on("mouseout", () => cbHover.current(null));

      setPronto(true);
      limpar = () => {
        m.remove();
        mapa.current = null;
        camada.current = null;
        cursor.current = null;
      };
    });
    return () => {
      cancelado = true;
      limpar?.();
      setPronto(false);
    };
  }, []);

  // Impressão: redesenha com as cores do tema claro.
  useEffect(() => {
    const antes = () => {
      setImpressao(true);
      mapa.current?.invalidateSize();
    };
    const depois = () => setImpressao(false);
    window.addEventListener("beforeprint", antes);
    window.addEventListener("afterprint", depois);
    return () => {
      window.removeEventListener("beforeprint", antes);
      window.removeEventListener("afterprint", depois);
    };
  }, []);

  // Rota, marcadores de início e fim, paradas e eventos.
  useEffect(() => {
    const lib = L.current;
    const grupo = camada.current;
    if (!pronto || !lib || !grupo) return;
    grupo.clearLayers();
    const claroForcado = impressao && tema !== "claro";
    const cor = (nome: string, claro: string) => (claroForcado ? claro : corCss(nome));
    const FAIXAS_CLARO: Record<string, string> = {
      parado: "#7b8794",
      ate_30: "#e8934a",
      "30_60": "#d9703a",
      "60_80": "#be5030",
      "80_110": "#983528",
      acima_110: "#6a1f22",
      falha_gps: "#7b8794",
      sem_velocidade: "#7b8794",
    };
    const corDaFaixa = (f: string) =>
      claroForcado
        ? (FAIXAS_CLARO[f] as string)
        : corCss(f === "falha_gps" || f === "sem_velocidade" ? "--faixa-falha" : `--faixa-${f}`);

    const contorno = corCss("--contorno-rota", "#ffffff");
    const segs = rota.segmentos.faixa;
    const trechos: { a: [number, number]; b: [number, number]; faixa: string }[] = [];
    for (let i = 0; i < segs.length; i++) {
      const la = rota.lat[i];
      const lo = rota.lon[i];
      const lb = rota.lat[i + 1];
      const ob = rota.lon[i + 1];
      if (
        la === null ||
        lo === null ||
        lb === null ||
        ob === null ||
        la === undefined ||
        lb === undefined
      )
        continue;
      trechos.push({ a: [la, lo as number], b: [lb, ob as number], faixa: segs[i] });
    }
    // Contorno branco por baixo, só onde há faixa de velocidade.
    for (const t of trechos) {
      if (t.faixa === "falha_gps" || t.faixa === "sem_velocidade") continue;
      lib
        .polyline([t.a, t.b], {
          color: contorno,
          weight: 8,
          opacity: 1,
          lineCap: "round",
          interactive: false,
        })
        .addTo(grupo);
    }
    for (const t of trechos) {
      const semDado = t.faixa === "falha_gps" || t.faixa === "sem_velocidade";
      lib
        .polyline([t.a, t.b], {
          color: corDaFaixa(t.faixa),
          weight: semDado ? 2 : 5,
          opacity: semDado ? 0.9 : 1,
          lineCap: "round",
          interactive: false,
        })
        .addTo(grupo);
    }

    for (const p of paradas) {
      if (p.lat === null || p.lon === null) continue;
      const raio = Math.min(14, Math.max(6, 6 + p.duracao_s * 0.35));
      lib
        .circleMarker([p.lat, p.lon], {
          radius: raio,
          color: cor("--text-secondary", "#46525f"),
          weight: 1.5,
          fillColor: cor("--faixa-parado", "#7b8794"),
          fillOpacity: 0.55,
        })
        .bindTooltip(`Parada · ${duracao(p.duracao_s)}`)
        .addTo(grupo);
    }

    for (const e of eventos) {
      if (e.lat === null || e.lon === null) continue;
      const icone = iconeEvento(lib, e.tipo);
      if (!icone) continue;
      lib
        .marker([e.lat, e.lon], {
          icon: icone,
          title: `${ROTULO_EVENTO[e.tipo]} às ${props.formatarT(e.t_inicio_ms)}`,
          alt: ROTULO_EVENTO[e.tipo],
          keyboard: true,
        })
        .addTo(grupo);
    }

    const validos = trechos.length ? trechos : [];
    const primeiro = validos[0]?.a ?? null;
    const ultimo = validos.length ? validos[validos.length - 1].b : null;
    if (primeiro)
      lib
        .marker(primeiro, { icon: iconeAB(lib, "A"), title: "Início da viagem", keyboard: false })
        .addTo(grupo);
    if (ultimo)
      lib
        .marker(ultimo, { icon: iconeAB(lib, "B"), title: "Fim da viagem", keyboard: false })
        .addTo(grupo);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pronto, rota, eventos, paradas, tema, impressao]);

  // Posição do cursor compartilhado.
  useEffect(() => {
    const lib = L.current;
    const m = mapa.current;
    if (!pronto || !lib || !m) return;
    const ponto = cursorT === null ? null : pontoDaRotaNoTempo(rota, cursorT);
    if (!ponto) {
      cursor.current?.remove();
      cursor.current = null;
      return;
    }
    if (!cursor.current) {
      cursor.current = lib
        .circleMarker([ponto.lat, ponto.lon], {
          radius: 7,
          color: corCss("--accent"),
          weight: 3,
          fillColor: corCss("--surface-card"),
          fillOpacity: 1,
          interactive: false,
        })
        .addTo(m);
    } else {
      cursor.current.setLatLng([ponto.lat, ponto.lon]);
    }
  }, [pronto, cursorT, rota, tema]);

  // "Ver no mapa": centraliza o evento.
  useEffect(() => {
    const m = mapa.current;
    if (!pronto || !m || !foco) return;
    m.setView([foco.lat, foco.lon], Math.max(m.getZoom(), 16), { animate: false });
  }, [pronto, foco]);

  // Tema: mostra só a camada do tema ativo (a clara também na impressão).
  useEffect(() => {
    const { claro, escuro } = tiles.current;
    const usarClaro = tema === "claro" || impressao;
    claro?.setOpacity(usarClaro ? 1 : 0);
    escuro?.setOpacity(usarClaro ? 0 : 1);
  }, [pronto, tema, impressao]);

  return (
    <div className={`mapa${semBase ? " mapa-sem-base" : ""}`}>
      <div
        ref={el}
        className="mapa-leaflet"
        role="application"
        aria-label="Mapa da rota colorida por velocidade"
      />
      {semBase ? <span className="mapa-aviso">Mapa-base indisponível</span> : null}
    </div>
  );
}
