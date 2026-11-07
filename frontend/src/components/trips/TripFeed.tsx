"use client";

import { useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import Link from "next/link";
import type { TripListItem } from "@/domain/trip/types";
import { formatNumber } from "@/format/numbers";
import type { useTrips } from "@/use-cases/useTrips";
import { Button, buttonClass } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Note } from "@/components/ui/Note";
import { IconReload } from "@/components/icons/Icons";
import { ConnectionBanner, ErrorState } from "@/components/analysis/States";
import { TripCard } from "./TripCard";
import { DeleteTripDialog } from "./DeleteTripDialog";

type Props = {
  query: ReturnType<typeof useTrips>;
  /** The search in force (already trimmed). */
  search: string;
  onClearSearch: () => void;
};

type FeedProps = Props & { onDelete: (trip: TripListItem) => void };

/** Distance below the last card at which the next batch is requested. */
const LOOKAHEAD = "600px 0px";
const SKELETONS = 4;

const GRID = "m-0 grid grid-cols-[repeat(auto-fill,minmax(min(480px,100%),1fr))] gap-4 p-0";

const never = () => () => undefined;
// `IntersectionObserver` is missing in old browsers and in test environments.
const supportsObserver = () => typeof IntersectionObserver !== "undefined";

function Skeleton() {
  return <div className="h-[132px] rounded-lg border border-hairline bg-track" />;
}

function Skeletons({ n }: { n: number }) {
  return Array.from({ length: n }, (_, i) => <Skeleton key={i} />);
}

/** The loaded trips in order, a trip that a refresh brought twice kept once. */
function uniqueTrips(data: ReturnType<typeof useTrips>["data"]): TripListItem[] {
  // Batches are cursor-based, but a refresh may still bring a trip twice: keep the first.
  const seen = new Map<string, TripListItem>();
  for (const page of data?.pages ?? []) {
    for (const trip of page.viagens) if (!seen.has(trip.viagem_id)) seen.set(trip.viagem_id, trip);
  }
  return [...seen.values()];
}

function Feed({ query, search, onClearSearch, onDelete }: FeedProps) {
  const {
    data,
    isPending,
    isError,
    isFetchNextPageError,
    isFetchingNextPage,
    isPlaceholderData,
    hasNextPage,
    fetchNextPage,
    refetch,
  } = query;

  const trips = useMemo(() => uniqueTrips(data), [data]);
  const total = data?.pages[0]?.total ?? 0;

  const observable = useSyncExternalStore(never, supportsObserver, () => true);
  const sentinel = useRef<HTMLDivElement>(null);
  const fetchMore = useRef(fetchNextPage);
  useEffect(() => {
    fetchMore.current = fetchNextPage;
  });

  // Only one request at a time, and none after a failure or while the list is a stale search.
  const canLoad = hasNextPage && !isFetchingNextPage && !isError && !isPlaceholderData;
  const loaded = trips.length;
  useEffect(() => {
    const el = sentinel.current;
    if (!canLoad || !observable || !el) return;
    const observer = new IntersectionObserver(
      (entries) => {
        if (!entries.some((e) => e.isIntersecting)) return;
        // Once per observer: a new one is made when the batch arrives, and reports again
        // if the end is still in sight.
        observer.disconnect();
        void fetchMore.current();
      },
      { rootMargin: LOOKAHEAD },
    );
    observer.observe(el);
    return () => observer.disconnect();
  }, [canLoad, observable, loaded]);

  if (isPending) {
    return (
      <div className={GRID} role="status" aria-busy="true" aria-label="Carregando viagens">
        <Skeletons n={SKELETONS} />
      </div>
    );
  }
  if (!data) {
    return (
      <ErrorState text="Não foi possível carregar as viagens." onRetry={() => void refetch()} />
    );
  }
  if (trips.length === 0) {
    return search ? (
      <Card roomy className="grid justify-items-start gap-4" role="status">
        <p className="m-0 type-title">Nenhuma viagem encontrada para «{search}»</p>
        <Button variant="secondary" onClick={onClearSearch}>
          Limpar busca
        </Button>
      </Card>
    ) : (
      <Card roomy className="grid justify-items-start gap-4" role="status">
        <p className="m-0 type-title">Nenhuma viagem ainda</p>
        <Link href="/processar" className={buttonClass()}>
          Processar viagem
        </Link>
      </Card>
    );
  }

  return (
    <>
      {isError && !isFetchNextPageError ? (
        <ConnectionBanner text="Não foi possível atualizar a lista. Mostrando o que já foi carregado." />
      ) : null}
      <div
        role="feed"
        aria-busy={isFetchingNextPage || isPlaceholderData}
        aria-label="Viagens"
        className={GRID}
      >
        {trips.map((trip, i) => (
          <TripCard
            key={trip.viagem_id}
            trip={trip}
            position={i + 1}
            size={total}
            onDelete={onDelete}
          />
        ))}
        {isFetchingNextPage ? <Skeletons n={SKELETONS} /> : null}
      </div>
      <div className="mt-5 grid justify-items-center gap-3">
        {isError && hasNextPage ? (
          <>
            <Note>Não foi possível carregar mais viagens.</Note>
            <Button icon={<IconReload />} onClick={() => void fetchNextPage()}>
              Tentar de novo
            </Button>
          </>
        ) : hasNextPage ? (
          observable ? (
            <div ref={sentinel} aria-hidden="true" className="h-px w-full" />
          ) : (
            <Button
              variant="secondary"
              disabled={isFetchingNextPage}
              onClick={() => void fetchNextPage()}
            >
              Carregar mais
            </Button>
          )
        ) : (
          <Note>
            {total === 1
              ? "A única viagem foi carregada."
              : `Todas as ${formatNumber(total)} viagens foram carregadas.`}
          </Note>
        )}
      </div>
    </>
  );
}

/**
 * The trip list with infinite scroll: the next batch loads as the end comes near. Each card's
 * trash button opens one confirmation dialog; after a deletion the focus moves to the next card
 * (or the previous one, for the last) and a status region announces it.
 */
export function TripFeed({ query, search, onClearSearch }: Props) {
  const [target, setTarget] = useState<TripListItem | null>(null);
  const [announcement, setAnnouncement] = useState("");
  const root = useRef<HTMLDivElement>(null);
  // The card that receives the focus once the deleted one has left the list.
  const focusId = useRef<string | null>(null);

  const askToDelete = (trip: TripListItem) => {
    const trips = uniqueTrips(query.data);
    const i = trips.findIndex((t) => t.viagem_id === trip.viagem_id);
    focusId.current = (trips[i + 1] ?? trips[i - 1])?.viagem_id ?? null;
    setAnnouncement("");
    setTarget(trip);
  };

  useEffect(() => {
    const id = focusId.current;
    if (!id || !announcement) return;
    focusId.current = null;
    root.current?.querySelector<HTMLElement>(`[data-trip-id="${id}"] a`)?.focus();
  });

  return (
    <div ref={root}>
      <Feed query={query} search={search} onClearSearch={onClearSearch} onDelete={askToDelete} />
      <DeleteTripDialog
        open={target !== null}
        tripId={target?.viagem_id ?? ""}
        driverName={target?.motorista.nome ?? null}
        returnTo={search ? `/viagens?busca=${encodeURIComponent(search)}` : "/viagens"}
        onClose={() => setTarget(null)}
        onDeleted={() => {
          setTarget(null);
          setAnnouncement("Viagem excluída");
        }}
      />
      <p role="status" className="sr-only">
        {announcement}
      </p>
    </div>
  );
}
