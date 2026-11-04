import { ProcessForm } from "@/components/process/ProcessForm";
import { Page } from "@/components/ui/Page";
import { PageHeader, PageHeaderTitle } from "@/components/ui/PageHeader";
import { ThemeToggle } from "@/components/ui/Theme";
import { requireSession } from "@/server/session";
import { initialDongle } from "@/server/trips/datasource";

export const dynamic = "force-dynamic";

export default async function ProcessPage() {
  await requireSession("/processar");
  return (
    <Page>
      <PageHeader>
        <PageHeaderTitle>
          <h1>Processar viagem</h1>
          <p className="text-secondary">Informe o motorista, o combustível e a viagem.</p>
        </PageHeaderTitle>
        <ThemeToggle />
      </PageHeader>
      <ProcessForm initialDongle={initialDongle()} />
    </Page>
  );
}
