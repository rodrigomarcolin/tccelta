// @vitest-environment node
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const auth = vi.fn();
vi.mock("@/auth", () => ({ auth: () => auth(), handlers: { GET: vi.fn(), POST: vi.fn() } }));

import { GET } from "./[estilo]/[z]/[x]/[y]/route";

const ctx = (estilo: string, z = "5", x = "10", y = "16") => ({
  params: Promise.resolve({ estilo, z, x, y }),
});
const req = () => new Request("http://x/api/mapa");

beforeEach(() => {
  auth.mockReset();
  auth.mockResolvedValue({ user: { email: "ana@usp.br" } });
  vi.stubEnv("OPERATOR_EMAILS", "ana@usp.br");
  vi.stubEnv("ANALISE_MAPA_KEY", "segredo");
});
afterEach(() => {
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

describe("proxy de tiles", () => {
  it("exige sessão", async () => {
    auth.mockResolvedValue(null);
    const f = vi.fn();
    vi.stubGlobal("fetch", f);
    expect((await GET(req(), ctx("claro"))).status).toBe(401);
    expect(f).not.toHaveBeenCalled();
  });

  it("rejeita estilo ou coordenada inválidos", async () => {
    const f = vi.fn();
    vi.stubGlobal("fetch", f);
    expect((await GET(req(), ctx("neon"))).status).toBe(404);
    expect((await GET(req(), ctx("claro", "5", "../x"))).status).toBe(404);
    expect(f).not.toHaveBeenCalled();
  });

  it("anexa a chave e devolve o tile", async () => {
    const f = vi
      .fn()
      .mockResolvedValue(new Response("png", { headers: { "content-type": "image/png" } }));
    vi.stubGlobal("fetch", f);
    const r = await GET(req(), ctx("escuro"));
    expect(r.status).toBe(200);
    expect(r.headers.get("content-type")).toBe("image/png");
    const url = String(f.mock.calls[0][0]);
    expect(url).toBe("https://basemaps.cartocdn.com/rastertiles/dark_all/5/10/16.png?key=segredo");
  });

  it("sem chave não envia o parâmetro", async () => {
    vi.stubEnv("ANALISE_MAPA_KEY", "");
    const f = vi.fn().mockResolvedValue(new Response("png"));
    vi.stubGlobal("fetch", f);
    await GET(req(), ctx("claro"));
    expect(String(f.mock.calls[0][0])).not.toContain("key=");
  });

  it("devolve 502 se a origem falha", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response("x", { status: 403 })));
    expect((await GET(req(), ctx("claro"))).status).toBe(502);
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("rede")));
    expect((await GET(req(), ctx("claro"))).status).toBe(502);
  });
});
