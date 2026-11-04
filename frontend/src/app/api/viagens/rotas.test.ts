// @vitest-environment node
import { beforeEach, describe, expect, it, vi } from "vitest";

const auth = vi.fn();
vi.mock("@/auth", () => ({ auth: () => auth(), handlers: { GET: vi.fn(), POST: vi.fn() } }));

import { reiniciarMock } from "@/servidor/viagens/mock/mock-cliente";
import { POST } from "./route";
import { GET as GET_STATUS } from "./[id]/route";
import { GET as GET_ANALISE } from "./[id]/analise/route";
import { EXEMPLOS } from "@/servidor/viagens/mock/cenarios";

const post = (corpo: string) =>
  POST(new Request("http://x/api/viagens", { method: "POST", body: corpo }));
const ctx = (id: string) => ({ params: Promise.resolve({ id }) });
const valido = JSON.stringify({
  motorista: { nome: "Rota" },
  categoria_viagem: "teste",
  combustivel: "diesel",
  entrada: { tipo: "boot", dongle_id: 1, boot_id: 8 },
});

beforeEach(() => {
  reiniciarMock();
  auth.mockReset();
});

describe("sem sessão", () => {
  beforeEach(() => auth.mockResolvedValue(null));

  it("POST responde 401 com problema", async () => {
    const r = await post(valido);
    expect(r.status).toBe(401);
    expect(r.headers.get("Content-Type")).toBe("application/problem+json");
    expect(r.headers.get("Cache-Control")).toBe("no-store");
    expect(await r.json()).toMatchObject({ codigo: "nao_autenticado", status: 401 });
  });

  it("GET status e análise respondem 401", async () => {
    const id = EXEMPLOS[0].id;
    expect((await GET_STATUS(new Request("http://x"), ctx(id))).status).toBe(401);
    expect((await GET_ANALISE(new Request("http://x"), ctx(id))).status).toBe(401);
  });
});

describe("com sessão", () => {
  beforeEach(() => auth.mockResolvedValue({ user: { email: "ana@usp.br" } }));

  it("POST cria, repassa status e corpo e não usa cache", async () => {
    const r = await post(valido);
    expect(r.status).toBe(202);
    expect(r.headers.get("Cache-Control")).toBe("no-store");
    expect(r.headers.get("Content-Type")).toBe("application/json");
    const corpo = (await r.json()) as { viagem_id: string };
    const s = await GET_STATUS(new Request("http://x"), ctx(corpo.viagem_id));
    expect(s.status).toBe(200);
    const a = await GET_ANALISE(new Request("http://x"), ctx(corpo.viagem_id));
    expect(a.status).toBe(409);
    expect(a.headers.get("Content-Type")).toBe("application/problem+json");
    expect(await a.json()).toMatchObject({ viagem_status: "pendente" });
  });

  it("repassa 409 de nome repetido", async () => {
    await post(valido);
    const r = await post(valido);
    expect(r.status).toBe(409);
    expect(await r.json()).toMatchObject({ codigo: "motorista_repetido" });
  });

  it("JSON inválido dá 400 corpo_malformado", async () => {
    const r = await post("{nao é json");
    expect(r.status).toBe(400);
    expect(await r.json()).toMatchObject({ codigo: "corpo_malformado" });
  });

  it("corpo acima de 4 KiB dá 413", async () => {
    const r = await post(JSON.stringify({ x: "a".repeat(5000) }));
    expect(r.status).toBe(413);
    expect(await r.json()).toMatchObject({ codigo: "corpo_grande" });
  });

  it("exemplo pronto devolve a análise", async () => {
    const r = await GET_ANALISE(new Request("http://x"), ctx(EXEMPLOS[0].id));
    expect(r.status).toBe(200);
    expect(await r.json()).toMatchObject({ versao_calculo: "1" });
  });

  it("id desconhecido dá 404", async () => {
    const r = await GET_STATUS(new Request("http://x"), ctx("zzz"));
    expect(r.status).toBe(404);
  });
});
