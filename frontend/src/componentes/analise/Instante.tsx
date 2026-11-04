import type { Instante as InstanteDados } from "@/dominio/analise";
import { rotuloMarcha } from "@/dominio/analise";
import { numero, TRACO } from "@/formato/numeros";

type Props = {
  instante: InstanteDados | null;
  formatarT: (t: number) => string;
  className?: string;
  /** Sem instante, não mostra a dica (usado sobre o mapa). */
  ocultarVazio?: boolean;
};

/** Tooltip fixo: hora, velocidade, rotação, marcha e pedal do instante mais próximo. */
export function TooltipInstante({ instante, formatarT, className, ocultarVazio }: Props) {
  if (!instante) {
    if (ocultarVazio) return null;
    return (
      <p className={`tooltip-instante tooltip-vazio ${className ?? ""}`} role="status">
        Passe o mouse sobre a rota ou os gráficos para ver o instante.
      </p>
    );
  }
  return (
    <p className={`tooltip-instante ${className ?? ""}`} role="status">
      <span className="dado">{formatarT(instante.t)}</span>
      <span>
        <span className="dado">{numero(instante.velocidade, 0)}</span>
        <span className="unidade">km/h</span>
      </span>
      <span>
        <span className="dado">{numero(instante.rpm, 0)}</span>
        <span className="unidade">rpm</span>
      </span>
      <span>
        Marcha{" "}
        <span className="dado">
          {instante.marcha === null ? TRACO : rotuloMarcha(instante.marcha)}
        </span>
      </span>
      <span>
        Pedal <span className="dado">{numero(instante.pedal, 0)}</span>
        <span className="unidade">%</span>
      </span>
    </p>
  );
}
