import type { ReactNode } from "react";

type Props = {
  rotulo: string;
  valor: ReactNode;
  unidade?: string;
  nota?: ReactNode;
};

/** Leitura pequena: rótulo em UI, valor em mono e unidade ao lado. */
export function Indicador({ rotulo, valor, unidade, nota }: Props) {
  return (
    <div className="indicador">
      <div className="indicador-rotulo">{rotulo}</div>
      <div className="indicador-valor">
        <span className="dado">{valor}</span>
        {unidade ? <span className="unidade">{unidade}</span> : null}
      </div>
      {nota ? <div className="indicador-nota">{nota}</div> : null}
    </div>
  );
}
