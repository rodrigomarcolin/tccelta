import type { Aviso } from "@/contrato/tipos";
import { IconeInfo } from "@/componentes/icones/Icones";
import { textoAviso } from "./textos";

export function FaixaAvisos({ avisos }: { avisos: Aviso[] }) {
  // `sem_leituras` vira o estado "Sem dados" no lugar do resumo.
  const lista = avisos.filter((a) => a.codigo !== "sem_leituras");
  if (lista.length === 0) return null;
  return (
    <ul className="faixa-avisos" aria-label="Avisos da análise">
      {lista.map((a, i) => (
        <li key={`${a.codigo}-${i}`} className="aviso">
          <IconeInfo />
          <span>{textoAviso(a)}</span>
        </li>
      ))}
    </ul>
  );
}
