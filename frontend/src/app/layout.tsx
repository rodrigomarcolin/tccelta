import type { Metadata, Viewport } from "next";
import { JetBrains_Mono, Space_Grotesk } from "next/font/google";
import { cookies } from "next/headers";
import type { ReactNode } from "react";
import { THEME_COOKIE, validTheme } from "@/config";
import { ThemeProvider } from "@/components/ui/Theme";
import { Providers } from "./providers";
import "@/styles/tokens/colors.css";
import "@/styles/tokens/typography.css";
import "@/styles/tokens/spacing.css";
import "@/styles/tokens/effects.css";
import "@/styles/globals.css";

const ui = Space_Grotesk({ subsets: ["latin"], variable: "--typeface-ui", display: "swap" });
const mono = JetBrains_Mono({ subsets: ["latin"], variable: "--typeface-mono", display: "swap" });

export const metadata: Metadata = {
  title: "Análise de viagem",
  description: "Análise de viagens do dongle OBD2",
  robots: { index: false, follow: false },
};

export const viewport: Viewport = { width: "device-width", initialScale: 1 };

// Reads the theme cookie on every request, to avoid flashing.
export const dynamic = "force-dynamic";

export default async function RootLayout({ children }: { children: ReactNode }) {
  const initial = validTheme((await cookies()).get(THEME_COOKIE)?.value);
  return (
    <html lang="pt-BR" data-theme={initial} className={`${ui.variable} ${mono.variable}`}>
      <body>
        <Providers>
          <ThemeProvider initial={initial}>{children}</ThemeProvider>
        </Providers>
      </body>
    </html>
  );
}
