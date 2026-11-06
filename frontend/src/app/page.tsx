import { redirect } from "next/navigation";
import { requireSession } from "@/server/session";

export const dynamic = "force-dynamic";

export default async function RootPage() {
  await requireSession("/");
  redirect("/processar");
}
