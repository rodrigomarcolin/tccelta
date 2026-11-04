import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/domain/trip/apiError";
import { createTrip, getTripAnalysis, getTripStatus } from "./tripDatasource";

type Resp = { status: number; body?: unknown; retryAfter?: string };

function stubFetch(resp: Resp | Error) {
  const f = vi.fn<typeof fetch>(async () => {
    if (resp instanceof Error) throw resp;
    return {
      status: resp.status,
      json: async () => {
        if (resp.body === undefined) throw new SyntaxError("no body");
        return resp.body;
      },
      headers: new Headers(resp.retryAfter ? { "Retry-After": resp.retryAfter } : {}),
    } as unknown as Response;
  });
  vi.stubGlobal("fetch", f);
  return f;
}

async function caught(promise: Promise<unknown>): Promise<ApiError> {
  try {
    await promise;
  } catch (error) {
    expect(error).toBeInstanceOf(ApiError);
    return error as ApiError;
  }
  throw new Error("did not throw");
}

const STATUS = { status: "pendente", motorista: { nome: "Maria" } };
const REQUEST = {
  motorista: { nome: "Maria" },
  categoria_viagem: "feira",
  combustivel: "gasolina_e30",
  entrada: { tipo: "boot" as const, dongle_id: 1, boot_id: 8 },
};

afterEach(() => vi.unstubAllGlobals());

describe("createTrip", () => {
  it("posts the JSON body without cache and returns the 202 body", async () => {
    const f = stubFetch({ status: 202, body: { viagem_id: "abc", status: "pendente" } });
    expect(await createTrip(REQUEST)).toEqual({ viagem_id: "abc", status: "pendente" });
    const [url, init] = f.mock.calls[0];
    expect(url).toBe("/api/trips");
    expect(init).toMatchObject({ method: "POST", cache: "no-store" });
    expect(JSON.parse(init?.body as string)).toEqual(REQUEST);
  });

  it("any other status throws ApiError with the problem", async () => {
    stubFetch({ status: 409, body: { codigo: "motorista_repetido", title: "t", status: 409 } });
    const error = await caught(createTrip(REQUEST));
    expect(error.status).toBe(409);
    expect(error.problem?.codigo).toBe("motorista_repetido");
    expect(error.retryAfter).toBeUndefined();
  });

  it("202 without viagem_id is off-contract", async () => {
    stubFetch({ status: 202, body: {} });
    const error = await caught(createTrip(REQUEST));
    expect(error.status).toBe(202);
    expect(error.problem).toBeNull();
  });

  it("a body that is not JSON gives no problem", async () => {
    stubFetch({ status: 500 });
    const error = await caught(createTrip(REQUEST));
    expect(error.status).toBe(500);
    expect(error.problem).toBeNull();
  });

  it("a network failure propagates as it is", async () => {
    const failure = new TypeError("Failed to fetch");
    stubFetch(failure);
    await expect(createTrip(REQUEST)).rejects.toBe(failure);
  });
});

describe("Retry-After", () => {
  it.each([
    ["5", 5],
    ["1.5", 1.5],
    ["0", undefined],
    ["-3", undefined],
    ["abc", undefined],
    ["Wed, 21 Oct 2026 07:28:00 GMT", undefined],
    [undefined, undefined],
  ])("%s becomes %s", async (header, expected) => {
    stubFetch({ status: 429, body: { codigo: "limite_taxa" }, retryAfter: header });
    expect((await caught(getTripStatus("t1"))).retryAfter).toBe(expected);
  });
});

describe("getTripStatus", () => {
  it("reads the status route with the signal and without cache", async () => {
    const f = stubFetch({ status: 200, body: STATUS });
    const controller = new AbortController();
    expect(await getTripStatus("t1", controller.signal)).toEqual(STATUS);
    const [url, init] = f.mock.calls[0];
    expect(url).toBe("/api/trips/t1");
    expect(init?.cache).toBe("no-store");
    expect(init?.signal).toBe(controller.signal);
  });

  it("keeps a problem that has only the codigo", async () => {
    stubFetch({ status: 502, body: { codigo: "credencial_invalida" }, retryAfter: "60" });
    const error = await caught(getTripStatus("t1"));
    expect(error.problem?.codigo).toBe("credencial_invalida");
    expect(error.retryAfter).toBe(60);
  });

  it.each([
    ["null body", null],
    ["no status", { motorista: { nome: "Maria" } }],
    ["no driver", { status: "pendente" }],
  ])("200 with %s is off-contract", async (_name, body) => {
    stubFetch({ status: 200, body });
    const error = await caught(getTripStatus("t1"));
    expect(error.status).toBe(200);
    expect(error.problem).toBeNull();
  });
});

describe("getTripAnalysis", () => {
  it("reads the analysis route", async () => {
    const f = stubFetch({ status: 200, body: { versao: 1 } });
    expect(await getTripAnalysis("t1")).toEqual({ versao: 1 });
    expect(f.mock.calls[0][0]).toBe("/api/trips/t1/analysis");
  });

  it("409 carries the problem", async () => {
    stubFetch({ status: 409, body: { codigo: "analise_indisponivel" } });
    const error = await caught(getTripAnalysis("t1"));
    expect(error.status).toBe(409);
    expect(error.problem?.codigo).toBe("analise_indisponivel");
  });

  it("200 with a null body is off-contract", async () => {
    stubFetch({ status: 200, body: null });
    expect((await caught(getTripAnalysis("t1"))).status).toBe(200);
  });
});
