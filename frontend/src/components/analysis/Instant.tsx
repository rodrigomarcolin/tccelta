import type { Instant as InstantData } from "@/domain/trip/analysis";
import { gearLabel } from "@/domain/trip/analysis";
import { formatNumber, DASH } from "@/format/numbers";
import { DataValue, Unit } from "@/components/ui/DataValue";

type Props = {
  instant: InstantData | null;
  formatT: (t: number) => string;
  className?: string;
  /** Without an instant, hides the hint (used over the map). */
  hideEmpty?: boolean;
};

/** Fixed tooltip: time, speed, RPM, gear and pedal of the nearest instant. */
const TOOLTIP =
  "m-0 mb-3 flex flex-wrap gap-5 rounded-md border border-hairline bg-card px-5 py-2 text-[13px] print:hidden";

export function InstantTooltip({ instant, formatT, className, hideEmpty }: Props) {
  if (!instant) {
    if (hideEmpty) return null;
    return (
      <p className={`${TOOLTIP} text-secondary ${className ?? ""}`} role="status">
        Passe o mouse sobre a rota ou os gráficos para ver o instante.
      </p>
    );
  }
  return (
    <p className={`${TOOLTIP} ${className ?? ""}`} role="status">
      <DataValue>{formatT(instant.t)}</DataValue>
      <span>
        <DataValue>{formatNumber(instant.speed, 0)}</DataValue>
        <Unit>km/h</Unit>
      </span>
      <span>
        <DataValue>{formatNumber(instant.rpm, 0)}</DataValue>
        <Unit>rpm</Unit>
      </span>
      <span>
        Marcha <DataValue>{instant.gear === null ? DASH : gearLabel(instant.gear)}</DataValue>
      </span>
      <span>
        Pedal <DataValue>{formatNumber(instant.pedal, 0)}</DataValue>
        <Unit>%</Unit>
      </span>
    </p>
  );
}
