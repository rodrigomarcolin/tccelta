import type { ReactNode } from "react";

type Props = {
  tom: "ao-vivo" | "aviso" | "alerta";
  pulsar?: boolean;
  children: ReactNode;
};

/** Pílula de estado. O ponto pulsa só enquanto há uma consulta em andamento. */
export function Distintivo({ tom, pulsar, children }: Props) {
  return (
    <span className={`distintivo distintivo-${tom}`}>
      <span className={`ponto${pulsar ? " ponto-pulsar" : ""}`} aria-hidden="true" />
      {children}
    </span>
  );
}
