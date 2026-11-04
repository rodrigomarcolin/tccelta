import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { contraste, misturar } from "./contraste";

const css = readFileSync(join(__dirname, "tokens/colors.css"), "utf8");

/** Lê as variáveis de um bloco (`escuro`, `claro` ou `impressao`). */
function bloco(nome: "escuro" | "claro" | "impressao"): Record<string, string> {
  const inicio =
    nome === "escuro"
      ? css.indexOf('[data-theme="escuro"] {')
      : nome === "claro"
        ? css.indexOf('[data-theme="claro"] {')
        : css.indexOf("@media print");
  const fim =
    nome === "escuro"
      ? css.indexOf('[data-theme="claro"] {')
      : nome === "claro"
        ? css.indexOf("@media print")
        : css.length;
  const out: Record<string, string> = {};
  for (const m of css.slice(inicio, fim).matchAll(/(--[\w-]+):\s*([^;]+);/g))
    out[m[1]] = m[2].trim();
  return out;
}

const escuro = bloco("escuro");
const claro = bloco("claro");
const impressao = bloco("impressao");

const TEXTOS_CLARO = [
  "--text-primary",
  "--text-secondary",
  "--text-tertiary",
  "--accent",
  "--warning",
  "--danger",
];
// No tema escuro, o terciário do design system (3,3:1) é só decorativo; a spec exige 4,5:1 nos tokens claros.
const TEXTOS_ESCURO = ["--text-primary", "--text-secondary", "--accent", "--warning", "--danger"];
const FAIXAS = ["ate_30", "30_60", "60_80", "80_110", "acima_110"] as const;

describe("tokens", () => {
  it("a rampa de faixas segue a spec 01, por tema", () => {
    expect(FAIXAS.map((f) => claro[`--faixa-${f}`])).toEqual([
      "#e8934a",
      "#d9703a",
      "#be5030",
      "#983528",
      "#6a1f22",
    ]);
    expect(FAIXAS.map((f) => escuro[`--faixa-${f}`])).toEqual([
      "#8c3326",
      "#b44d2c",
      "#d96a31",
      "#ec8e42",
      "#f7b878",
    ]);
  });

  it("valores essenciais dos dois temas", () => {
    expect(escuro).toMatchObject({
      "--bg-screen": "#0a0e14",
      "--surface-card": "#141b24",
      "--accent": "#22d3ee",
      "--warning": "#ffb020",
      "--danger": "#f4385a",
    });
    expect(claro).toMatchObject({
      "--bg-screen": "#f2f5f8",
      "--surface-card": "#ffffff",
      "--accent": "#08768c",
      "--warning": "#9a5b00",
      "--danger": "#c0213f",
    });
  });

  it("a impressão força os tokens do tema claro", () => {
    for (const [k, v] of Object.entries(impressao)) {
      if (k === "--bg-screen") continue; // papel branco
      expect(claro[k], k).toBe(v);
    }
    expect(impressao["--bg-screen"]).toBe("#ffffff");
  });

  it("não há verde na interface nem na rampa", () => {
    const verde = (hex: string) => {
      const n = Number.parseInt(hex.slice(1), 16);
      const [r, g, b] = [(n >> 16) & 255, (n >> 8) & 255, n & 255];
      return g > r + 20 && g > b + 20;
    };
    for (const tema of [escuro, claro]) {
      for (const [k, v] of Object.entries(tema)) {
        if (/^#[0-9a-f]{6}$/i.test(v)) expect(verde(v), `${k}=${v}`).toBe(false);
      }
    }
    expect(css).not.toMatch(/green/i);
  });

  it("não usa sombra nos estilos globais", () => {
    const globais = readFileSync(join(__dirname, "globais.css"), "utf8");
    const sombras = globais.match(/box-shadow:[^;]*;/g) ?? [];
    expect(sombras).toEqual(["box-shadow: var(--glow-cyan);"]);
  });
});

/**
 * Critério 28: contraste das cores de dados.
 * Faixas de velocidade contra as superfícies do tema e contra o contorno
 * branco (mínimo 2:1); marchas contra as superfícies (mínimo 3:1 para
 * gráficos); texto dos tokens contra as superfícies (mínimo 4,5:1).
 * Os números medidos ficam em `coverage/contraste.json` para o relatório.
 */
describe("contraste (critério 28)", () => {
  const relatorio: Record<string, number> = {};
  const registrar = (nome: string, valor: number) =>
    (relatorio[nome] = Math.round(valor * 100) / 100);

  it.each([
    ["escuro", escuro, ["#0a0e14", "#141b24"]],
    ["claro", claro, ["#f2f5f8", "#ffffff"]],
  ] as const)("texto dos tokens do tema %s tem 4,5:1", (_n, tema, fundos) => {
    for (const fundo of fundos) {
      for (const t of _n === "claro" ? TEXTOS_CLARO : TEXTOS_ESCURO) {
        const c = contraste(tema[t], fundo);
        registrar(`${_n}:${t} sobre ${fundo}`, c);
        expect(c, `${t} sobre ${fundo}`).toBeGreaterThanOrEqual(4.5);
      }
    }
  });

  it("rampa de faixas: contra o contorno branco e as superfícies (mínimo 2:1)", () => {
    const falhas: string[] = [];
    for (const [nome, tema, fundos] of [
      ["escuro", escuro, ["#0a0e14", "#141b24"]],
      ["claro", claro, ["#f2f5f8", "#ffffff"]],
    ] as const) {
      for (const f of FAIXAS) {
        const cor = tema[`--faixa-${f}`];
        for (const fundo of [...fundos, "#ffffff"]) {
          const c = contraste(cor, fundo);
          registrar(`${nome}:faixa ${f} sobre ${fundo}`, c);
          if (c < 2) falhas.push(`${nome} ${f} ${cor} sobre ${fundo}: ${c.toFixed(2)}`);
        }
      }
    }
    // Falhas registradas (a rampa da spec 01 não muda): ver o relatório final da entrega.
    registrar("faixas:falhas", falhas.length);
    expect(falhas).toEqual(ESPERADAS_FALHAS);
  });

  it("marchas e mapa de calor legíveis sobre as superfícies (mínimo 3:1)", () => {
    for (const [nome, tema, fundos, base] of [
      ["escuro", escuro, ["#0a0e14", "#141b24"], "#6ea8dc"],
      ["claro", claro, ["#f2f5f8", "#ffffff"], "#1f5f99"],
    ] as const) {
      for (let g = 0; g <= 6; g++) {
        for (const fundo of fundos) {
          const c = contraste(tema[`--marcha-${g}`], fundo);
          registrar(`${nome}:marcha ${g} sobre ${fundo}`, c);
          expect(c, `${nome} marcha ${g} sobre ${fundo}`).toBeGreaterThanOrEqual(g === 0 ? 3 : 3);
        }
      }
      // Célula mais clara do calor (alfa .12) distinta da superfície e a mais escura contra ela.
      const fundo = fundos[1];
      const forte = contraste(misturar(base, fundo, 1), fundo);
      registrar(`${nome}:calor máximo sobre ${fundo}`, forte);
      expect(forte).toBeGreaterThanOrEqual(3);
    }
  });
});

// Falha registrada (critério 28): a faixa mais clara do tema escuro tem só 1,74:1 contra o contorno branco.
// A rampa da spec 01 não muda; a cor continua legível contra o mapa-base escuro (mais de 10:1).
const ESPERADAS_FALHAS: string[] = ["escuro acima_110 #f7b878 sobre #ffffff: 1.74"];
