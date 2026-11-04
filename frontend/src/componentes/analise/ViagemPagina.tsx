"use client";

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import { POLLING_ESPERA_MS, POLLING_PROVISORIA_MAX_MS, POLLING_PROVISORIA_MS } from "@/config";
import type { Analise, StatusResposta } from "@/contrato/tipos";
import { AlternarTema } from "@/componentes/ui/Tema";
import { AnaliseView } from "./AnaliseView";
import {
  EstadoCarregando,
  EstadoErro,
  EstadoEspera,
  EstadoFalhou,
  EstadoNaoEncontrada,
} from "./Estados";

type Fase = "carregando" | "espera" | "falhou" | "nao_encontrada" | "erro" | "pronta";

type Estado = {
  fase: Fase;
  motorista: string | null;
  analise: Analise | null;
  provisoria: boolean;
};

const INICIAL: Estado = { fase: "carregando", motorista: null, analise: null, provisoria: false };

type Props = {
  id: string;
  /** Troca a navegação (testes). */
  irPara?: (url: string) => void;
};

function irParaPadrao(url: string) {
  window.location.assign(url);
}

function PaginaSimples({ children }: { children: ReactNode }) {
  return (
    <div className="analise">
      <header className="cabecalho">
        <h1>Análise de viagem</h1>
        <AlternarTema />
      </header>
      {children}
    </div>
  );
}

/** Espera, polling e exibição da análise de `/viagem/[id]`. */
export function ViagemPagina({ id, irPara = irParaPadrao }: Props) {
  const [estado, setEstado] = useState<Estado>(INICIAL);
  const [tentativa, setTentativa] = useState(0);
  const irParaRef = useRef(irPara);
  useEffect(() => {
    irParaRef.current = irPara;
  });

  useEffect(() => {
    let ativo = true;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let inicioProvisoria: number | null = null;
    let temAnalise = false;
    let provisoriaAtual = false;
    const controle = new AbortController();

    const sessaoExpirou = () =>
      irParaRef.current(`/entrar?retorno=${encodeURIComponent(`/viagem/${id}`)}`);

    const agendar = (ms: number) => {
      if (ativo) timer = setTimeout(() => void ciclo(), ms);
    };

    async function buscarAnalise(provisoria: boolean): Promise<"ok" | "indisponivel" | "erro"> {
      const r = await fetch(`/api/viagens/${id}/analise`, {
        cache: "no-store",
        signal: controle.signal,
      });
      if (r.status === 401) {
        sessaoExpirou();
        return "erro";
      }
      if (r.status === 409) return "indisponivel";
      if (!r.ok) return "erro";
      const analise = (await r.json()) as Analise;
      if (!ativo) return "ok";
      temAnalise = true;
      setEstado((e) => ({ ...e, fase: "pronta", analise, provisoria }));
      return "ok";
    }

    async function ciclo() {
      try {
        const r = await fetch(`/api/viagens/${id}`, { cache: "no-store", signal: controle.signal });
        if (!ativo) return;
        if (r.status === 401) return sessaoExpirou();
        if (r.status === 404) return setEstado({ ...INICIAL, fase: "nao_encontrada" });
        if (!r.ok) return setEstado((e) => ({ ...e, fase: "erro" }));
        const st = (await r.json()) as StatusResposta;
        if (!ativo) return;
        const motorista = st.motorista.nome;

        if (st.status === "pendente" || st.status === "aguardando_dados") {
          setEstado((e) => ({ ...e, fase: "espera", motorista }));
          return agendar(POLLING_ESPERA_MS);
        }
        if (st.status === "falhou") return setEstado({ ...INICIAL, fase: "falhou", motorista });

        // Pronta: busca a análise se ainda não tem ou se deixou de ser provisória.
        const deixouDeSerProvisoria = provisoriaAtual && !st.provisoria;
        if (!temAnalise || deixouDeSerProvisoria) {
          const resultado = await buscarAnalise(st.provisoria);
          if (!ativo) return;
          if (resultado === "erro") return setEstado((e) => ({ ...e, fase: "erro", motorista }));
          if (resultado === "indisponivel") return agendar(POLLING_ESPERA_MS);
        }
        provisoriaAtual = st.provisoria;
        setEstado((e) => ({ ...e, motorista, provisoria: st.provisoria }));

        if (st.provisoria) {
          inicioProvisoria ??= Date.now();
          if (Date.now() - inicioProvisoria < POLLING_PROVISORIA_MAX_MS) {
            agendar(POLLING_PROVISORIA_MS);
          }
        }
      } catch (erro) {
        if (!ativo || (erro instanceof DOMException && erro.name === "AbortError")) return;
        setEstado((e) => ({ ...e, fase: "erro" }));
      }
    }

    void ciclo();
    return () => {
      ativo = false;
      controle.abort();
      if (timer) clearTimeout(timer);
    };
  }, [id, tentativa]);

  const tentarDeNovo = useCallback(() => {
    setEstado(INICIAL);
    setTentativa((n) => n + 1);
  }, []);

  switch (estado.fase) {
    case "carregando":
      return (
        <PaginaSimples>
          <EstadoCarregando />
        </PaginaSimples>
      );
    case "espera":
      return (
        <PaginaSimples>
          <EstadoEspera motorista={estado.motorista} />
        </PaginaSimples>
      );
    case "falhou":
      return (
        <PaginaSimples>
          <EstadoFalhou />
        </PaginaSimples>
      );
    case "nao_encontrada":
      return (
        <PaginaSimples>
          <EstadoNaoEncontrada />
        </PaginaSimples>
      );
    case "erro":
      return (
        <PaginaSimples>
          <EstadoErro onTentar={tentarDeNovo} />
        </PaginaSimples>
      );
    case "pronta":
      return estado.analise ? (
        <AnaliseView analise={estado.analise} provisoria={estado.provisoria} />
      ) : null;
  }
}
