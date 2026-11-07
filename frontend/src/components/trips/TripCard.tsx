import Link from "next/link";
import type { TripListItem } from "@/domain/trip/types";
import {
  DASH,
  dayMonth,
  formatDuration,
  formatNumber,
  formatTime,
  fullDate,
} from "@/format/numbers";
import { Badge } from "@/components/ui/Badge";
import { DataValue } from "@/components/ui/DataValue";
import { IconTrash } from "@/components/icons/Icons";
import { RoutePreview } from "./RoutePreview";

/** Height of a card; also what the browser assumes for a card it has not rendered yet. */
export const CARD_HEIGHT = 132;

function whenText(trip: TripListItem): string {
  const start = trip.inicio_em ? Date.parse(trip.inicio_em) : NaN;
  if (Number.isNaN(start)) {
    const created = Date.parse(trip.criada_em);
    return `criada ${fullDate(created)} ${formatTime(created)}`;
  }
  const end = trip.fim_em ? Date.parse(trip.fim_em) : NaN;
  const first = `${fullDate(start)} ${formatTime(start)}`;
  if (Number.isNaN(end)) return first;
  // An end on another day carries its date.
  const sameDay = fullDate(end) === fullDate(start);
  return `${first}–${sameDay ? "" : `${dayMonth(end)} `}${formatTime(end)}`;
}

function metricsText(trip: TripListItem): string | null {
  if (trip.status !== "pronta") return null;
  const parts = [
    trip.distancia_km === null ? null : `${formatNumber(trip.distancia_km, 1)} km`,
    trip.duracao_s === null ? null : formatDuration(trip.duracao_s),
  ].filter((p): p is string => p !== null);
  return parts.length ? parts.join(" · ") : "Sem dados";
}

function sourceText(trip: TripListItem): string {
  if (trip.entrada_tipo === "intervalo") return `Dongle ${trip.dongle_id} · intervalo`;
  const generation = trip.boot_generation ? ` · ger. ${trip.boot_generation}` : "";
  return `Dongle ${trip.dongle_id} · boot ${trip.boot_id ?? DASH}${generation}`;
}

function StatusBadge({ trip }: { trip: TripListItem }) {
  if (trip.status === "pendente" || trip.status === "aguardando_dados") {
    return (
      <Badge tone="live" pulse>
        Processando
      </Badge>
    );
  }
  if (trip.status === "falhou") return <Badge tone="alert">Falhou</Badge>;
  if (trip.provisoria) return <Badge tone="warning">Provisória</Badge>;
  return null;
}

/**
 * One trip of the list: minimap on the left, text on the right, the whole card a link to the
 * analysis. Horizontal on every screen width, so more trips fit on a phone too. The delete button
 * is the link's sibling (a button inside a link is invalid HTML), over its top-right corner and always
 * visible, so it works on a phone without hover.
 */
export function TripCard({
  trip,
  position,
  size,
  onDelete,
}: {
  trip: TripListItem;
  position: number;
  size: number;
  /** The delete button was pressed; the confirmation is the parent's. */
  onDelete: (trip: TripListItem) => void;
}) {
  const metrics = metricsText(trip);
  return (
    <article
      aria-posinset={position}
      aria-setsize={size}
      data-trip-id={trip.viagem_id}
      // Cards off screen skip rendering; the remembered size keeps the scrollbar from jumping.
      className="relative [contain-intrinsic-size:auto_132px] [content-visibility:auto]"
    >
      <Link
        href={`/viagem/${trip.viagem_id}`}
        prefetch={false}
        className="flex h-[132px] overflow-hidden rounded-lg border border-hairline bg-card text-primary no-underline transition-[border-color] duration-(--dur-color) ease-value hover:border-strong focus-visible:outline-offset-[-2px]"
      >
        <div className="w-[220px] shrink-0 border-r border-hairline phone:w-[110px]">
          <RoutePreview route={trip.rota_resumo} status={trip.status} />
        </div>
        <div className="grid min-w-0 flex-1 content-center gap-0.5 py-[10px] pr-14 pl-4">
          <div className="flex min-w-0 items-center gap-2">
            <span className="min-w-0 truncate type-title">{trip.motorista.nome}</span>
            <span className="shrink-0">
              <StatusBadge trip={trip} />
            </span>
          </div>
          <span className="truncate text-[13px] text-secondary">{whenText(trip)}</span>
          {metrics ? <DataValue className="truncate text-[13px]">{metrics}</DataValue> : null}
          <DataValue className="truncate text-[13px] text-secondary">{sourceText(trip)}</DataValue>
          <span className="truncate text-[12px] text-secondary">
            categoria {trip.categoria_viagem}
          </span>
        </div>
      </Link>
      <button
        type="button"
        onClick={() => onDelete(trip)}
        aria-label={`Excluir a viagem de ${trip.motorista.nome}`}
        className="absolute top-2 right-2 grid size-9 cursor-pointer place-items-center rounded-md border-0 bg-transparent text-secondary transition-[background,color] duration-(--dur-color) ease-value hover:bg-danger-wash hover:text-danger phone:size-11"
      >
        <IconTrash />
      </button>
    </article>
  );
}
