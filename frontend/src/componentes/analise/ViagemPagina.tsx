"use client";

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import {
  FUSO,
  LIMITE_ESPERA_MS,
  POLLING_ESPERA_MS,
  POLLING_FALHAS_AVISO,
  POLLING_PROVISORIA_MAX_MS,
  POLLING_PROVISORIA_MS,
  POLLING_RECUO_MAX_MS,
} from "@/config";
import { ehProblema, type Analise, type StatusResposta } from "@/contrato/tipos";
import { AlternarTema } from "@/componentes/ui/Tema";
import { AnaliseView } from "./AnaliseView";
import {
  EstadoCarregando,
  EstadoErro,
  EstadoEspera,
  EstadoFalhou,
  EstadoLimiteEspera,
  EstadoNaoEncontrada,
  FaixaConexao,
} from "./Estados";

type Fase = "carregando" | "espera" | "limite" | "falhou" | "nao_encontrada" | "erro" | "pronta";

type Estado = {
  fase: Fase;
  motorista: string | null;
  analise: Analise | null;
  provisoria: boolean;
  provisoriaExpirada: boolean;
  /** Aviso de conexão ou de servidor ocupado, sem esvaziar a tela. */
  aviso: string | null;
  /** HH:MM da última troca de versão da análise. */
  atualizadoEm: string | null;
};

const INICIAL: Estado = {
  fase: "carregando",
  motorista: null,
  analise: null,
  provisoria: false,
  provisoriaExpirada: false,
  aviso: null,
  atualizadoEm: null,
};

const AVISO_REDE = "Sem conexão com o servidor. Tentando novamente…";

const AVISO_CREDENCIAL = "O servidor não aceitou a credencial do sistema. Chame a organização.";
const avisoBloqueado = (s: number) =>
  `Acesso temporariamente bloqueado. Tente de novo em ${Math.ceil(s)} s.`;

const formatarHora = (iso: string): string | null => {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return null;
  return new Intl.DateTimeFormat("pt-BR", {
    timeZone: FUSO,
    hour: "2-digit",
    minute: "2-digit",
  }).format(d);
};

type Props = {
  id: string;
  /** Troca a navegação (testes). */
  irPara?: (url: string) => void;
};

function irParaPadrao(url: string) {
  window.location.assign(url);
}

function PaginaSimples({ children, aviso }: { children: ReactNode; aviso?: string | null }) {
  return (
    <div className="analise">
      <header className="cabecalho">
        <h1>Análise de viagem</h1>
        <AlternarTema />
      </header>
      {aviso ? <FaixaConexao texto={aviso} /> : null}
      {children}
    </div>
  );
}

type Lida = { status: number; corpo: unknown; retryAfter?: number };

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
    let parou = false;
    let emVoo = false;
    /** Há um ciclo adiado porque a aba está oculta. */
    let pausado = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let falhas = 0;
    let inicioProvisoria: number | null = null;
    let temAnalise = false;
    let calculadaAtual: string | null = null;
    const limiteEspera = Date.now() + LIMITE_ESPERA_MS;
    const controle = new AbortController();

    const mudar = (parcial: Partial<Estado>) => setEstado((e) => ({ ...e, ...parcial }));
    const limpar = () => {
      if (timer !== undefined) clearTimeout(timer);
      timer = undefined;
    };
    const parar = () => {
      parou = true;
      limpar();
    };
    const agendar = (ms: number) => {
      limpar();
      pausado = false;
      if (!ativo || parou) return;
      if (document.hidden) {
        pausado = true;
        return;
      }
      timer = setTimeout(() => void ciclo(), ms);
    };

    const sessaoExpirou = () => {
      parar();
      irParaRef.current(`/entrar?retorno=${encodeURIComponent(`/viagem/${id}`)}`);
    };

    /** Falha de rede ou 5xx: conta, recua (1, 2, 4, até 5 s) e avisa na terceira seguida. */
    const falha = () => {
      falhas += 1;
      if (falhas >= POLLING_FALHAS_AVISO) mudar({ aviso: AVISO_REDE });
      agendar(Math.min(POLLING_RECUO_MAX_MS, POLLING_ESPERA_MS * 2 ** (falhas - 1)));
    };

    const ocupado = (retryAfter: number | undefined) => {
      const s = Math.max(1, retryAfter ?? 1);
      mudar({ aviso: `Servidor ocupado. Tentando de novo em ${s} s.` });
      agendar(s * 1000);
    };

    const erroInesperado = () => {
      if (temAnalise) {
        falha();
        return;
      }
      parar();
      mudar({ fase: "erro", aviso: null });
    };

    const ler = async (url: string): Promise<Lida> => {
      const r = await fetch(url, { cache: "no-store", signal: controle.signal });
      const corpo: unknown = await r.json().catch(() => null);
      const ra = Number(r.headers?.get("Retry-After"));
      return {
        status: r.status,
        corpo,
        retryAfter: Number.isFinite(ra) && ra > 0 ? ra : undefined,
      };
    };

    /** Trata o que é comum às duas rotas. Devolve `true` se já decidiu o que fazer. */
    const falhaComum = (r: Lida): boolean => {
      // Credencial do sistema recusada ou acesso bloqueado: o servidor já segurou
      // as tentativas até o prazo; o polling pausa por esse tempo e não insiste.
      const codigo = ehProblema(r.corpo) ? r.corpo.codigo : undefined;
      if (codigo === "credencial_invalida" || codigo === "bloqueado") {
        const s = Math.max(1, r.retryAfter ?? 60);
        mudar({ aviso: codigo === "bloqueado" ? avisoBloqueado(s) : AVISO_CREDENCIAL });
        agendar(s * 1000);
        return true;
      }
      if (r.status === 401) {
        sessaoExpirou();
        return true;
      }
      if (r.status === 429) {
        ocupado(r.retryAfter);
        return true;
      }
      if (r.status >= 500) {
        falha();
        return true;
      }
      return false;
    };

    async function passo() {
      const st = await ler(`/api/viagens/${id}`);
      if (!ativo || falhaComum(st)) return;
      if (st.status === 404) {
        parar();
        setEstado({ ...INICIAL, fase: "nao_encontrada" });
        return;
      }
      const corpo = st.corpo as StatusResposta | null;
      if (st.status !== 200 || !corpo || typeof corpo.status !== "string") {
        erroInesperado();
        return;
      }
      falhas = 0;
      const motorista = corpo.motorista.nome;

      if (corpo.status === "pendente" || corpo.status === "aguardando_dados") {
        if (Date.now() >= limiteEspera) {
          parar();
          mudar({ fase: "limite", motorista, aviso: null });
          return;
        }
        mudar({ fase: "espera", motorista, aviso: null });
        agendar(POLLING_ESPERA_MS);
        return;
      }
      if (corpo.status === "falhou") {
        parar();
        setEstado({ ...INICIAL, fase: "falhou", motorista });
        return;
      }

      // Pronta: busca a análise se ainda não tem ou se `calculada_em` mudou.
      const calculada = corpo.calculada_em;
      const mudou = temAnalise && calculada !== calculadaAtual;
      let atualizadoEm: string | undefined;
      if (!temAnalise || mudou) {
        const a = await ler(`/api/viagens/${id}/analise`);
        if (!ativo || falhaComum(a)) return;
        if (a.status === 409) {
          agendar(POLLING_ESPERA_MS);
          return;
        }
        if (a.status !== 200 || !a.corpo) {
          erroInesperado();
          return;
        }
        if (mudou && calculada) atualizadoEm = formatarHora(calculada) ?? undefined;
        temAnalise = true;
        calculadaAtual = calculada;
        mudar({ analise: a.corpo as Analise, fase: "pronta" });
      }
      falhas = 0;
      setEstado((e) => ({
        ...e,
        motorista,
        provisoria: corpo.provisoria,
        aviso: null,
        atualizadoEm: atualizadoEm ?? e.atualizadoEm,
      }));

      if (!corpo.provisoria) {
        parar();
        return;
      }
      inicioProvisoria ??= Date.now();
      if (Date.now() - inicioProvisoria < POLLING_PROVISORIA_MAX_MS) {
        agendar(POLLING_PROVISORIA_MS);
        return;
      }
      parar();
      mudar({ provisoriaExpirada: true });
    }

    async function ciclo() {
      if (!ativo || parou || emVoo) return;
      emVoo = true;
      try {
        await passo();
      } catch {
        if (ativo) falha();
      } finally {
        emVoo = false;
      }
    }

    const aoMudarVisibilidade = () => {
      if (parou) return;
      if (document.hidden) {
        if (timer !== undefined) {
          limpar();
          pausado = true;
        }
      } else if (pausado) {
        pausado = false;
        void ciclo();
      }
    };
    const aoVoltarRede = () => {
      if (parou || emVoo) return;
      limpar();
      pausado = false;
      void ciclo();
    };
    document.addEventListener("visibilitychange", aoMudarVisibilidade);
    window.addEventListener("online", aoVoltarRede);

    void ciclo();
    return () => {
      ativo = false;
      controle.abort();
      limpar();
      document.removeEventListener("visibilitychange", aoMudarVisibilidade);
      window.removeEventListener("online", aoVoltarRede);
    };
  }, [id, tentativa]);

  const tentarDeNovo = useCallback(() => {
    setEstado(INICIAL);
    setTentativa((n) => n + 1);
  }, []);

  const continuarEsperando = useCallback(() => {
    setEstado((e) => ({ ...e, fase: "espera" }));
    setTentativa((n) => n + 1);
  }, []);

  switch (estado.fase) {
    case "carregando":
      return (
        <PaginaSimples aviso={estado.aviso}>
          <EstadoCarregando />
        </PaginaSimples>
      );
    case "espera":
      return (
        <PaginaSimples aviso={estado.aviso}>
          <EstadoEspera motorista={estado.motorista} />
        </PaginaSimples>
      );
    case "limite":
      return (
        <PaginaSimples>
          <EstadoLimiteEspera onContinuar={continuarEsperando} />
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
        <AnaliseView
          analise={estado.analise}
          provisoria={estado.provisoria}
          provisoriaExpirada={estado.provisoriaExpirada}
          aviso={estado.aviso}
          atualizadoEm={estado.atualizadoEm}
        />
      ) : null;
  }
}
