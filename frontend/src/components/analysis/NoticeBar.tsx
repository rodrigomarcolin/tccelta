import type { TripWarning } from "@/domain/trip/types";
import { IconInfo } from "@/components/icons/Icons";
import { warningText } from "./texts";

export function NoticeBar({ warnings }: { warnings: TripWarning[] }) {
  // `sem_leituras` becomes the "Sem dados" state in place of the summary.
  const list = warnings.filter((a) => a.codigo !== "sem_leituras");
  if (list.length === 0) return null;
  return (
    <ul className="m-0 mb-5 grid list-none gap-2 p-0" aria-label="Avisos da análise">
      {list.map((a, i) => (
        <li
          key={`${a.codigo}-${i}`}
          className="flex items-center gap-3 rounded-md border border-hairline bg-warning-wash px-5 py-3 text-primary"
        >
          <IconInfo className="flex-none text-warning" />
          <span>{warningText(a)}</span>
        </li>
      ))}
    </ul>
  );
}
