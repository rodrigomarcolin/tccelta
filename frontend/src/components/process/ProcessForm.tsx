"use client";

import { useRef, useState, type FormEvent } from "react";
import { FUELS, INITIAL_DONGLE } from "@/config";
import { ApiError } from "@/domain/trip/apiError";
import type { CreateTripRequest, Problem } from "@/domain/trip/types";
import { useCreateTrip } from "@/use-cases/useCreateTrip";
import { Button } from "@/components/ui/Button";

type Fields = {
  driver: string;
  category: string;
  fuel: string;
  dongle: string;
  mode: "boot" | "interval";
  bootId: string;
  bootGeneration: string;
  t0: string;
  t1: string;
};

type Errors = Partial<
  Record<"driver" | "category" | "dongle" | "bootId" | "bootGeneration" | "t0" | "t1", string>
> & {
  general?: string;
  /** The session expired: the form stays and the login opens in another tab. */
  sessionExpired?: boolean;
  /** The next submit is a new attempt (the `POST` never repeats on its own). */
  retry?: boolean;
};

const initialFields = (dongle: number): Fields => ({
  driver: "",
  category: "feira",
  fuel: "gasolina_e30",
  dongle: String(dongle),
  mode: "boot",
  bootId: "",
  bootGeneration: "",
  t0: "",
  t1: "",
});

/** `datetime-local` becomes RFC 3339 with the fair fixed timezone (-03:00). */
export function toRfc3339(value: string): string {
  const full = value.length === 16 ? `${value}:00` : value;
  return `${full}-03:00`;
}

export function validateFields(c: Fields): Errors {
  const e: Errors = {};
  if (!c.driver.trim()) e.driver = "Informe o nome do motorista.";
  if (!c.category.trim()) e.category = "Informe a categoria.";
  if (!c.dongle.trim() || !Number.isInteger(Number(c.dongle)) || Number(c.dongle) < 1) {
    e.dongle = "Informe o número do dongle.";
  }
  if (c.mode === "boot") {
    if (!c.bootId.trim() || !Number.isInteger(Number(c.bootId)) || Number(c.bootId) < 1) {
      e.bootId = "Informe o boot.";
    }
    if (
      c.bootGeneration.trim() &&
      (!Number.isInteger(Number(c.bootGeneration)) || Number(c.bootGeneration) < 0)
    ) {
      e.bootGeneration = "A geração deve ser um número inteiro.";
    }
  } else {
    if (!c.t0) e.t0 = "Informe o início.";
    if (!c.t1) e.t1 = "Informe o fim.";
    if (c.t0 && c.t1 && Date.parse(toRfc3339(c.t1)) <= Date.parse(toRfc3339(c.t0))) {
      e.t1 = "O fim deve ser depois do início.";
    }
  }
  return e;
}

export function buildBody(c: Fields): CreateTripRequest {
  const dongle = Number(c.dongle);
  return {
    motorista: { nome: c.driver.trim() },
    categoria_viagem: c.category.trim(),
    combustivel: c.fuel,
    entrada:
      c.mode === "boot"
        ? {
            tipo: "boot",
            dongle_id: dongle,
            boot_id: Number(c.bootId),
            ...(c.bootGeneration.trim() ? { boot_generation: Number(c.bootGeneration) } : {}),
          }
        : { tipo: "intervalo", dongle_id: dongle, t0: toRfc3339(c.t0), t1: toRfc3339(c.t1) },
  };
}

type Props = {
  /** Initial value of the dongle field (`ANALISE_DONGLE_ID` on the server). */
  initialDongle?: number;
  goTo?: (url: string) => void;
};

/** Translates the error response of the `POST` (by `codigo`, never by `title` or `detail`). */
export function creationError(
  status: number,
  p: Problem | null,
  retryAfter: number | null,
  driver: string,
  category: string,
): Errors {
  const code = p?.codigo;
  if (status === 409 && code === "motorista_repetido") {
    return {
      driver: `Já existe uma viagem de «${driver}» na categoria «${category.toLowerCase()}». Use outro nome.`,
    };
  }
  if (status === 400 && code === "entrada_invalida") {
    return { general: p?.detail ?? "Entrada inválida." };
  }
  if (status === 404 && code === "dongle_desconhecido") {
    return { dongle: "Dongle não cadastrado." };
  }
  if (status === 404 && code === "boot_desconhecido") {
    return { bootId: "Este boot não existe para o dongle informado. Confira o número." };
  }
  if (code === "credencial_invalida") {
    return { general: "O servidor não aceitou a credencial do sistema. Chame a organização." };
  }
  if (code === "bloqueado") {
    const s = retryAfter ?? 0;
    return {
      general: `Acesso temporariamente bloqueado. Tente de novo em ${Number.isFinite(s) && s > 0 ? `${Math.ceil(s)} s` : "instantes"}.`,
      retry: true,
    };
  }
  if (status === 401) return { general: "Sessão expirada.", sessionExpired: true };
  if (status === 403) return { general: "Esta conta não tem permissão de operador." };
  if (status === 429) {
    const s = retryAfter ?? 0;
    return {
      general:
        Number.isFinite(s) && s > 0
          ? `Servidor ocupado. Tente de novo em ${Math.ceil(s)} s.`
          : "Servidor ocupado. Tente de novo em instantes.",
      retry: true,
    };
  }
  if (status === 503 && code === "api_indisponivel") {
    return { general: "Não foi possível falar com o servidor.", retry: true };
  }
  if (status === 503) return { general: "Servidor sobrecarregado.", retry: true };
  return { general: "Erro inesperado no servidor.", retry: true };
}

export function ProcessForm({
  initialDongle = INITIAL_DONGLE,
  goTo = (u) => window.location.assign(u),
}: Props) {
  const driverRef = useRef<HTMLInputElement>(null);
  const [c, setC] = useState<Fields>(() => initialFields(initialDongle));
  const [errors, setErrors] = useState<Errors>({});
  const createTrip = useCreateTrip();
  // Stays disabled after a 202, during navigation.
  const submitting = createTrip.isPending || createTrip.isSuccess;

  const change =
    <K extends keyof Fields>(field: K) =>
    (value: Fields[K]) =>
      setC((current) => ({ ...current, [field]: value }));

  async function submit(ev: FormEvent) {
    ev.preventDefault();
    if (submitting) return;
    const localErrors = validateFields(c);
    setErrors(localErrors);
    if (Object.keys(localErrors).length > 0) return;

    try {
      const created = await createTrip.mutateAsync(buildBody(c));
      goTo(`/viagem/${created.viagem_id}`);
    } catch (error) {
      if (!(error instanceof ApiError)) {
        setErrors({ general: "Não foi possível falar com o servidor.", retry: true });
        return;
      }
      const next = creationError(
        error.status,
        error.problem,
        error.retryAfter ?? null,
        c.driver.trim(),
        c.category.trim(),
      );
      setErrors(next);
      if (next.driver) driverRef.current?.focus();
    }
  }

  const field = (id: keyof Errors, label: string, input: React.ReactNode) => (
    <div className="field">
      <label htmlFor={`f-${id}`}>{label}</label>
      {input}
      {errors[id] ? (
        <p className="field-error" id={`error-${id}`} role="alert">
          {errors[id]}
        </p>
      ) : null}
    </div>
  );
  const ariaError = (id: keyof Errors) =>
    errors[id] ? { "aria-invalid": true as const, "aria-describedby": `error-${id}` } : {};

  return (
    <form className="form card" onSubmit={(e) => void submit(e)} noValidate>
      {field(
        "driver",
        "Motorista",
        <input
          id="f-driver"
          ref={driverRef}
          value={c.driver}
          maxLength={60}
          autoComplete="off"
          onChange={(e) => change("driver")(e.target.value)}
          {...ariaError("driver")}
        />,
      )}
      {field(
        "category",
        "Categoria",
        <input
          id="f-category"
          value={c.category}
          maxLength={40}
          autoComplete="off"
          onChange={(e) => change("category")(e.target.value)}
          {...ariaError("category")}
        />,
      )}
      <div className="field">
        <label htmlFor="f-fuel">Combustível</label>
        <select id="f-fuel" value={c.fuel} onChange={(e) => change("fuel")(e.target.value)}>
          {FUELS.map((o) => (
            <option key={o.id} value={o.id}>
              {o.name}
            </option>
          ))}
        </select>
      </div>
      {field(
        "dongle",
        "Dongle",
        <input
          id="f-dongle"
          type="number"
          min={1}
          inputMode="numeric"
          value={c.dongle}
          onChange={(e) => change("dongle")(e.target.value)}
          {...ariaError("dongle")}
        />,
      )}

      <fieldset className="field">
        <legend>Viagem definida por</legend>
        <div className="options">
          <label className="option">
            <input
              type="radio"
              name="mode"
              checked={c.mode === "boot"}
              onChange={() => change("mode")("boot")}
            />
            Boot
          </label>
          <label className="option">
            <input
              type="radio"
              name="mode"
              checked={c.mode === "interval"}
              onChange={() => change("mode")("interval")}
            />
            Intervalo de tempo
          </label>
        </div>
      </fieldset>

      {c.mode === "boot" ? (
        <div className="field-row">
          {field(
            "bootId",
            "Boot",
            <input
              id="f-bootId"
              type="number"
              min={1}
              inputMode="numeric"
              value={c.bootId}
              onChange={(e) => change("bootId")(e.target.value)}
              {...ariaError("bootId")}
            />,
          )}
          {field(
            "bootGeneration",
            "Geração do boot (opcional)",
            <input
              id="f-bootGeneration"
              type="number"
              min={0}
              inputMode="numeric"
              value={c.bootGeneration}
              onChange={(e) => change("bootGeneration")(e.target.value)}
              {...ariaError("bootGeneration")}
            />,
          )}
        </div>
      ) : (
        <div className="field-row">
          {field(
            "t0",
            "Início",
            <input
              id="f-t0"
              type="datetime-local"
              value={c.t0}
              onChange={(e) => change("t0")(e.target.value)}
              {...ariaError("t0")}
            />,
          )}
          {field(
            "t1",
            "Fim",
            <input
              id="f-t1"
              type="datetime-local"
              value={c.t1}
              onChange={(e) => change("t1")(e.target.value)}
              {...ariaError("t1")}
            />,
          )}
        </div>
      )}

      {errors.general ? (
        <p className="field-error form-error" role="alert">
          {errors.general}
          {errors.sessionExpired ? (
            <>
              {" "}
              <a
                href={`/entrar?retorno=${encodeURIComponent("/processar")}`}
                target="_blank"
                rel="noopener noreferrer"
              >
                Entrar de novo
              </a>
              {" (abre em outra aba; depois envie de novo)."}
            </>
          ) : null}
        </p>
      ) : null}
      <Button type="submit" fullWidth disabled={submitting}>
        {submitting ? "Enviando…" : errors.retry ? "Tentar de novo" : "Processar viagem"}
      </Button>
    </form>
  );
}
