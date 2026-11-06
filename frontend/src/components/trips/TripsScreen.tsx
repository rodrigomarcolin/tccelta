"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { SEARCH_DEBOUNCE_MS, SEARCH_MAX_LENGTH } from "@/config";
import { ApiError } from "@/domain/trip/apiError";
import { formatNumber } from "@/format/numbers";
import { useTrips } from "@/use-cases/useTrips";
import { HeaderControls } from "@/components/ui/AppNav";
import { Input } from "@/components/ui/Field";
import { PageHeader, PageHeaderTitle } from "@/components/ui/PageHeader";
import { TripFeed } from "./TripFeed";

const listUrl = (search: string) =>
  search ? `/viagens?busca=${encodeURIComponent(search)}` : "/viagens";

/**
 * `/viagens`: fixed header (title, navigation, search) over the infinite list. Only the
 * search lives in the URL; the loaded batches live in the query cache, so coming back from
 * a trip shows them at once.
 */
export function TripsScreen({ search }: { search: string }) {
  const router = useRouter();
  const query = useTrips(search);
  const [text, setText] = useState(search);
  const [seenSearch, setSeenSearch] = useState(search);
  // The search last pushed to the URL, so the debounce does not push it twice.
  const [pushed, setPushed] = useState(search);

  // The URL changed from outside (back button), not by our own push: the field follows.
  if (seenSearch !== search) {
    setSeenSearch(search);
    if (search !== pushed) {
      setText(search);
      setPushed(search);
    }
  }

  const push = (next: string) => {
    setPushed(next);
    window.scrollTo({ top: 0 });
    router.replace(listUrl(next));
  };

  useEffect(() => {
    const next = text.trim();
    if (next === pushed) return;
    const timer = setTimeout(() => {
      setPushed(next);
      window.scrollTo({ top: 0 });
      router.replace(listUrl(next));
    }, SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [text, pushed, router]);

  const sessionExpired = query.error instanceof ApiError && query.error.status === 401;
  useEffect(() => {
    if (sessionExpired) router.replace(`/entrar?retorno=${encodeURIComponent(listUrl(search))}`);
  }, [sessionExpired, search, router]);

  const total = query.data?.pages[0]?.total;
  const count =
    total === undefined
      ? "Carregando…"
      : `${total === 1 ? "1 viagem" : `${formatNumber(total)} viagens`}${search ? " encontradas" : ""}`;

  return (
    <>
      <PageHeader sticky stackOnPhone>
        <PageHeaderTitle>
          <h1>Viagens</h1>
          <p className="m-0 text-secondary">
            {count} · Mapas ©{" "}
            <a
              href="https://www.openstreetmap.org/copyright"
              target="_blank"
              rel="noopener noreferrer"
            >
              OpenStreetMap
            </a>
          </p>
        </PageHeaderTitle>
        <div className="grid min-w-0 justify-items-end gap-3 phone:w-full phone:justify-items-start">
          <HeaderControls current="viagens" />
          <form
            role="search"
            className="w-[300px] max-w-full phone:w-full print:hidden"
            onSubmit={(e) => e.preventDefault()}
          >
            <Input
              type="search"
              aria-label="Buscar motorista"
              placeholder="Buscar motorista…"
              autoComplete="off"
              maxLength={SEARCH_MAX_LENGTH}
              value={text}
              onChange={(e) => setText(e.target.value)}
              className="w-full"
            />
          </form>
        </div>
      </PageHeader>
      <TripFeed
        query={query}
        search={search}
        onClearSearch={() => {
          setText("");
          push("");
        }}
      />
    </>
  );
}
