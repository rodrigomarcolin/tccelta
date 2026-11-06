import "server-only";
import { redirect } from "next/navigation";
import { auth } from "@/auth";

/** Checks the session in a server component; without one, goes to /entrar and comes back afterwards. */
export async function requireSession(path: string): Promise<void> {
  const session = await auth();
  if (!session?.user) redirect(`/entrar?retorno=${encodeURIComponent(path)}`);
}
