import { TripsScreen } from "@/components/trips/TripsScreen";
import { Page } from "@/components/ui/Page";
import { SEARCH_MAX_LENGTH } from "@/config";
import { requireSession } from "@/server/session";

export const dynamic = "force-dynamic";

/** The only thing the URL carries is the search: `/viagens?busca=joao`. */
export default async function TripsRoute({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>;
}) {
  const raw = (await searchParams).busca;
  const search = [...(Array.isArray(raw) ? (raw[0] ?? "") : (raw ?? "")).trim()]
    .slice(0, SEARCH_MAX_LENGTH)
    .join("")
    .trim();
  await requireSession(search ? `/viagens?busca=${encodeURIComponent(search)}` : "/viagens");
  return (
    <Page wide>
      <TripsScreen search={search} />
    </Page>
  );
}
