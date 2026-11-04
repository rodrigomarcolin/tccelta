// @vitest-environment node
import { existsSync, readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

/**
 * Bundle do navegador (INT2). Depois de `next build` com um segredo sentinela
 * (`ANALISE_CLIENT_SECRET=$SENTINELA npm run build`), nada em `.next/static`
 * pode trazer os nomes das variáveis, o valor do segredo ou um JWT. Sem build,
 * o teste é ignorado.
 */
const ESTATICO = join(process.cwd(), ".next", "static");
const SENTINELA = process.env.ANALISE_CLIENT_SECRET ?? "sentinela-segredo-int2-nao-vazar";
const ID_SENTINELA = process.env.ANALISE_CLIENT_ID ?? "sentinela-id-int2";

function arquivos(dir: string): string[] {
  return readdirSync(dir).flatMap((nome) => {
    const caminho = join(dir, nome);
    return statSync(caminho).isDirectory() ? arquivos(caminho) : [caminho];
  });
}

describe.skipIf(!existsSync(ESTATICO))("bundle entregue ao navegador", () => {
  const texto = existsSync(ESTATICO)
    ? arquivos(ESTATICO)
        .filter((a) => /\.(js|css|html|json|txt|map)$/.test(a))
        .map((a) => readFileSync(a, "utf8"))
        .join("\n")
    : "";

  it("tem conteúdo para varrer", () => {
    expect(texto.length).toBeGreaterThan(1000);
  });

  it.each(["ANALISE_CLIENT_SECRET", "ANALISE_CLIENT_ID", "NEXT_PUBLIC_ANALISE", "grant_type"])(
    "não contém %s",
    (nome) => {
      expect(texto).not.toContain(nome);
    },
  );

  it("não contém o valor do segredo nem do client_id", () => {
    expect(texto).not.toContain(SENTINELA);
    expect(texto).not.toContain(ID_SENTINELA);
  });

  it("não contém um JWT", () => {
    expect(texto).not.toMatch(/eyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}/);
  });
});
