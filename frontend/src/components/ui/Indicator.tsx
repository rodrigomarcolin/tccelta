import type { ReactNode } from "react";

type Props = {
  label: string;
  value: ReactNode;
  unit?: string;
  note?: ReactNode;
};

/** Small reading: label in UI font, value in mono and unit alongside. */
export function Indicator({ label, value, unit, note }: Props) {
  return (
    <div className="indicator">
      <div className="indicator-label">{label}</div>
      <div className="indicator-value">
        <span className="data-value">{value}</span>
        {unit ? <span className="unit">{unit}</span> : null}
      </div>
      {note ? <div className="indicator-note">{note}</div> : null}
    </div>
  );
}
