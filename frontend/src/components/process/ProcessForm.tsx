"use client";

import { useRef, useState, type FormEvent } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { FUELS, INITIAL_DONGLE } from "@/config";
import { ApiError } from "@/domain/trip/apiError";
import type { CreateTripRequest, Problem } from "@/domain/trip/types";
import { useCreateTrip } from "@/use-cases/useCreateTrip";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import {
  Field,
  FieldError,
  FieldLabel,
  FieldRow,
  FieldSet,
  Input,
  Radio,
  Select,
} from "@/components/ui/Field";

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
  /** The trip already exists (`409 viagem_repetida`): offers to open it. */
  repeated?: { id: string | null };
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
  if (status === 409 && code === "viagem_repetida") {
    // The name is the one the existing trip was processed for, not the one just typed.
    const owner = p?.motorista?.nome ?? driver;
    return {
      general: `Esta viagem já foi processada para «${owner}».`,
      repeated: { id: typeof p?.viagem_id === "string" && p.viagem_id ? p.viagem_id : null },
    };
  }
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

export function ProcessForm({ initialDongle = INITIAL_DONGLE }: Props) {
  const router = useRouter();
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
      router.push(`/viagem/${created.viagem_id}`);
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

  const field = (id: Exclude<keyof Errors, "repeated">, label: string, input: React.ReactNode) => (
    <Field>
      <FieldLabel htmlFor={`f-${id}`}>{label}</FieldLabel>
      {input}
      {errors[id] ? <FieldError id={`error-${id}`}>{errors[id]}</FieldError> : null}
    </Field>
  );
  const ariaError = (id: Exclude<keyof Errors, "repeated">) =>
    errors[id] ? { "aria-invalid": true as const, "aria-describedby": `error-${id}` } : {};

  return (
    <Card as="form" avoidBreak className="grid gap-5" onSubmit={(e) => void submit(e)} noValidate>
      {field(
        "driver",
        "Motorista",
        <Input
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
        <Input
          id="f-category"
          value={c.category}
          maxLength={40}
          autoComplete="off"
          onChange={(e) => change("category")(e.target.value)}
          {...ariaError("category")}
        />,
      )}
      <Field>
        <FieldLabel htmlFor="f-fuel">Combustível</FieldLabel>
        <Select id="f-fuel" value={c.fuel} onChange={(e) => change("fuel")(e.target.value)}>
          {FUELS.map((o) => (
            <option key={o.id} value={o.id}>
              {o.name}
            </option>
          ))}
        </Select>
      </Field>
      {field(
        "dongle",
        "Dongle",
        <Input
          mono
          id="f-dongle"
          type="number"
          min={1}
          inputMode="numeric"
          value={c.dongle}
          onChange={(e) => change("dongle")(e.target.value)}
          {...ariaError("dongle")}
        />,
      )}

      <FieldSet legend="Viagem definida por">
        <div className="flex gap-7">
          <Radio
            label="Boot"
            name="mode"
            checked={c.mode === "boot"}
            onChange={() => change("mode")("boot")}
          />
          <Radio
            label="Intervalo de tempo"
            name="mode"
            checked={c.mode === "interval"}
            onChange={() => change("mode")("interval")}
          />
        </div>
      </FieldSet>

      {c.mode === "boot" ? (
        <FieldRow>
          {field(
            "bootId",
            "Boot",
            <Input
              mono
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
            <Input
              mono
              id="f-bootGeneration"
              type="number"
              min={0}
              inputMode="numeric"
              value={c.bootGeneration}
              onChange={(e) => change("bootGeneration")(e.target.value)}
              {...ariaError("bootGeneration")}
            />,
          )}
        </FieldRow>
      ) : (
        <FieldRow>
          {field(
            "t0",
            "Início",
            <Input
              mono
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
            <Input
              mono
              id="f-t1"
              type="datetime-local"
              value={c.t1}
              onChange={(e) => change("t1")(e.target.value)}
              {...ariaError("t1")}
            />,
          )}
        </FieldRow>
      )}

      {errors.general ? (
        <FieldError box>
          {errors.general}
          {errors.repeated ? (
            <>
              {" "}
              {errors.repeated.id ? (
                <>
                  <Link href={`/viagem/${encodeURIComponent(errors.repeated.id)}`}>
                    Abrir a viagem
                  </Link>
                  {" · "}
                </>
              ) : null}
              <Link href="/viagens">Ver todas as viagens</Link>
            </>
          ) : null}
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
        </FieldError>
      ) : null}
      <Button type="submit" fullWidth disabled={submitting}>
        {submitting ? "Enviando…" : errors.retry ? "Tentar de novo" : "Processar viagem"}
      </Button>
    </Card>
  );
}
