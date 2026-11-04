import { redirect } from "next/navigation";
import { exigirSessao } from "@/servidor/sessao";

export const dynamic = "force-dynamic";

export default async function Raiz() {
  await exigirSessao("/");
  redirect("/processar");
}
