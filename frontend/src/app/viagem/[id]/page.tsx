import { TripPage } from "@/components/analysis/TripPage";
import { requireSession } from "@/server/session";

export const dynamic = "force-dynamic";

export default async function TripRoute({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  await requireSession(`/viagem/${id}`);
  return (
    <main className="page-wide">
      <TripPage id={id} />
    </main>
  );
}
