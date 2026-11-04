"use client";

import { createContext, useCallback, useContext, useState, type ReactNode } from "react";
import { TEMA_COOKIE, type Tema } from "@/config";
import { IconeLua, IconeSol } from "@/componentes/icones/Icones";

const Contexto = createContext<Tema>("escuro");

export function useTema(): Tema {
  return useContext(Contexto);
}

export function gravarTema(tema: Tema, doc: Document = document): void {
  doc.documentElement.dataset.theme = tema;
  doc.cookie = `${TEMA_COOKIE}=${tema}; path=/; max-age=31536000; samesite=lax`;
}

/** Tema inicial vem do cookie lido no servidor, para não piscar. */
export function TemaProvider({ inicial, children }: { inicial: Tema; children: ReactNode }) {
  const [tema, setTema] = useState<Tema>(inicial);
  const alternar = useCallback(
    (novo: Tema) => {
      gravarTema(novo);
      setTema(novo);
    },
    [setTema],
  );
  return (
    <Contexto.Provider value={tema}>
      <AlternarContexto.Provider value={alternar}>{children}</AlternarContexto.Provider>
    </Contexto.Provider>
  );
}

const AlternarContexto = createContext<(t: Tema) => void>(() => undefined);

export function AlternarTema() {
  const tema = useTema();
  const alternar = useContext(AlternarContexto);
  const claro = tema === "claro";
  return (
    <button
      type="button"
      className="botao botao-secundario botao-icone"
      aria-pressed={claro}
      aria-label="Tema claro"
      title={claro ? "Voltar ao tema escuro" : "Usar tema claro"}
      onClick={() => alternar(claro ? "escuro" : "claro")}
    >
      {claro ? <IconeSol /> : <IconeLua />}
      <span className="texto-botao-tema">Tema claro</span>
    </button>
  );
}
