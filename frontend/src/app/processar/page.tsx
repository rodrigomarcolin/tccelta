import { ProcessForm } from "@/components/process/ProcessForm";
import { ThemeToggle } from "@/components/ui/Theme";
import { requireSession } from "@/server/session";
import { initialDongle } from "@/server/trips/datasource";

export const dynamic = "force-dynamic";

export default async function ProcessPage() {
  await requireSession("/processar");
  return (
    <main className="page-narrow">
      <header className="header">
        <div className="header-title">
          <h1>Processar viagem</h1>
          <p className="text-secondary">Informe o motorista, o combustível e a viagem.</p>
        </div>
        <ThemeToggle />
      </header>
      <ProcessForm initialDongle={initialDongle()} />
    </main>
  );
}
