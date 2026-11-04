import type { Instant as InstantData } from "@/domain/trip/analysis";
import { gearLabel } from "@/domain/trip/analysis";
import { formatNumber, DASH } from "@/format/numbers";

type Props = {
  instant: InstantData | null;
  formatT: (t: number) => string;
  className?: string;
  /** Without an instant, hides the hint (used over the map). */
  hideEmpty?: boolean;
};

/** Fixed tooltip: time, speed, RPM, gear and pedal of the nearest instant. */
export function InstantTooltip({ instant, formatT, className, hideEmpty }: Props) {
  if (!instant) {
    if (hideEmpty) return null;
    return (
      <p className={`instant-tooltip tooltip-empty ${className ?? ""}`} role="status">
        Passe o mouse sobre a rota ou os gráficos para ver o instante.
      </p>
    );
  }
  return (
    <p className={`instant-tooltip ${className ?? ""}`} role="status">
      <span className="data-value">{formatT(instant.t)}</span>
      <span>
        <span className="data-value">{formatNumber(instant.speed, 0)}</span>
        <span className="unit">km/h</span>
      </span>
      <span>
        <span className="data-value">{formatNumber(instant.rpm, 0)}</span>
        <span className="unit">rpm</span>
      </span>
      <span>
        Marcha{" "}
        <span className="data-value">{instant.gear === null ? DASH : gearLabel(instant.gear)}</span>
      </span>
      <span>
        Pedal <span className="data-value">{formatNumber(instant.pedal, 0)}</span>
        <span className="unit">%</span>
      </span>
    </p>
  );
}
