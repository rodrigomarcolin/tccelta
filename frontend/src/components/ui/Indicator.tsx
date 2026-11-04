import type { ReactNode } from "react";
import { Card } from "./Card";
import { DataValue, Unit } from "./DataValue";

type Props = {
  label: string;
  value: ReactNode;
  unit?: string;
  note?: ReactNode;
};

/** Small reading: label in UI font, value in mono and unit alongside. */
export function Indicator({ label, value, unit, note }: Props) {
  return (
    <div>
      <div className="text-[12px] text-secondary">{label}</div>
      <div className="text-[20px]">
        <DataValue>{value}</DataValue>
        {unit ? <Unit>{unit}</Unit> : null}
      </div>
      {note ? <div className="text-[12px] text-secondary">{note}</div> : null}
    </div>
  );
}

/** Card with a row of indicators that wraps to fit the width. */
export function IndicatorRow({ children }: { children: ReactNode }) {
  return (
    <Card className="mb-5 grid grid-cols-[repeat(auto-fit,minmax(170px,1fr))] gap-4">
      {children}
    </Card>
  );
}
