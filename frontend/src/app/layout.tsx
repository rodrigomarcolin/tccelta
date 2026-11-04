import type { Metadata, Viewport } from "next";
import { JetBrains_Mono, Space_Grotesk } from "next/font/google";
import { cookies } from "next/headers";
import type { ReactNode } from "react";
import { TEMA_COOKIE, temaValido } from "@/config";
import { TemaProvider } from "@/componentes/ui/Tema";
import "@/estilos/tokens/colors.css";
import "@/estilos/tokens/typography.css";
import "@/estilos/tokens/spacing.css";
import "@/estilos/tokens/effects.css";
import "@/estilos/globais.css";

const ui = Space_Grotesk({ subsets: ["latin"], variable: "--fonte-ui", display: "swap" });
const mono = JetBrains_Mono({ subsets: ["latin"], variable: "--fonte-mono", display: "swap" });

export const metadata: Metadata = {
  title: "Análise de viagem",
  description: "Análise de viagens do dongle OBD2",
  robots: { index: false, follow: false },
};

export const viewport: Viewport = { width: "device-width", initialScale: 1 };

// Lê o cookie de tema a cada requisição, para não piscar.
export const dynamic = "force-dynamic";

export default async function RootLayout({ children }: { children: ReactNode }) {
  const tema = temaValido((await cookies()).get(TEMA_COOKIE)?.value);
  return (
    <html lang="pt-BR" data-theme={tema} className={`${ui.variable} ${mono.variable}`}>
      <body>
        <TemaProvider inicial={tema}>{children}</TemaProvider>
      </body>
    </html>
  );
}
