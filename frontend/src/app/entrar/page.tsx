import { redirect } from "next/navigation";
import { auth, signIn } from "@/auth";
import { safeReturnPath } from "@/auth/operators";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { FieldError } from "@/components/ui/Field";
import { BASE_PATH } from "@/config";
import { Page } from "@/components/ui/Page";
import { IconDongle } from "@/components/icons/Icons";

export const dynamic = "force-dynamic";

const MESSAGES: Record<string, string> = {
  conta_nao_liberada:
    "Esta conta não está liberada para processar viagens. Use um e-mail USP autorizado.",
};

export default async function SignInPage({
  searchParams,
}: {
  searchParams: Promise<{ erro?: string; retorno?: string; error?: string }>;
}) {
  const { erro: errorCode, retorno: returnTo, error } = await searchParams;
  const destination = safeReturnPath(returnTo);
  const session = await auth();
  if (session?.user) redirect(destination);

  const message = errorCode
    ? (MESSAGES[errorCode] ?? "Não foi possível entrar. Tente de novo.")
    : error
      ? "Não foi possível entrar. Tente de novo."
      : null;

  async function signInWithGoogle() {
    "use server";
    // Auth.js builds this URL itself (no Next basePath handling), unlike redirect() above.
    await signIn("google", { redirectTo: `${BASE_PATH}${destination}` });
  }

  return (
    <Page>
      <Card avoidBreak className="mt-[12vh] grid justify-items-start gap-4">
        <IconDongle size={28} />
        <h1>Análise de viagem</h1>
        <p className="text-secondary">Acesso restrito a operadores com e-mail USP autorizado.</p>
        {message ? <FieldError>{message}</FieldError> : null}
        <form action={signInWithGoogle}>
          <Button type="submit" fullWidth>
            Entrar com Google
          </Button>
        </form>
      </Card>
    </Page>
  );
}
