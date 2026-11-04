import type { TripWarning } from "@/domain/trip/types";
import { IconInfo } from "@/components/icons/Icons";
import { warningText } from "./texts";

export function NoticeBar({ warnings }: { warnings: TripWarning[] }) {
  // `sem_leituras` becomes the "Sem dados" state in place of the summary.
  const list = warnings.filter((a) => a.codigo !== "sem_leituras");
  if (list.length === 0) return null;
  return (
    <ul className="notice-bar" aria-label="Avisos da análise">
      {list.map((a, i) => (
        <li key={`${a.codigo}-${i}`} className="notice">
          <IconInfo />
          <span>{warningText(a)}</span>
        </li>
      ))}
    </ul>
  );
}
