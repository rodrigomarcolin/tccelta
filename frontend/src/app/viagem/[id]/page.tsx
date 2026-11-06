import { TripPage } from "@/components/analysis/TripPage";
import { Page } from "@/components/ui/Page";
import { requireSession } from "@/server/session";

export const dynamic = "force-dynamic";

export default async function TripRoute({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  await requireSession(`/viagem/${id}`);
  return (
    <Page wide>
      <TripPage id={id} />
    </Page>
  );
}
