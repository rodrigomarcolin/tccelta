"use client";

import { useState, type FormEvent } from "react";
import { COMBUSTIVEIS, DONGLE_INICIAL } from "@/config";
import type { CriarViagemResposta, Problema } from "@/contrato/tipos";
import { Botao } from "@/componentes/ui/Botao";

type Campos = {
  motorista: string;
  categoria: string;
  combustivel: string;
  dongle: string;
  tipo: "boot" | "intervalo";
  bootId: string;
  bootGeneration: string;
  t0: string;
  t1: string;
};

type Erros = Partial<
  Record<"motorista" | "categoria" | "dongle" | "bootId" | "bootGeneration" | "t0" | "t1", string>
> & {
  geral?: string;
};

const INICIAL: Campos = {
  motorista: "",
  categoria: "feira",
  combustivel: "gasolina_e30",
  dongle: String(DONGLE_INICIAL),
  tipo: "boot",
  bootId: "",
  bootGeneration: "",
  t0: "",
  t1: "",
};

/** `datetime-local` vira RFC 3339 com o fuso fixo da feira (-03:00). */
export function paraRfc3339(valor: string): string {
  const completo = valor.length === 16 ? `${valor}:00` : valor;
  return `${completo}-03:00`;
}

export function validarCampos(c: Campos): Erros {
  const e: Erros = {};
  if (!c.motorista.trim()) e.motorista = "Informe o nome do motorista.";
  if (!c.categoria.trim()) e.categoria = "Informe a categoria.";
  if (!c.dongle.trim() || !Number.isInteger(Number(c.dongle)) || Number(c.dongle) < 1) {
    e.dongle = "Informe o número do dongle.";
  }
  if (c.tipo === "boot") {
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
    if (c.t0 && c.t1 && Date.parse(paraRfc3339(c.t1)) <= Date.parse(paraRfc3339(c.t0))) {
      e.t1 = "O fim deve ser depois do início.";
    }
  }
  return e;
}

export function montarCorpo(c: Campos) {
  const dongle = Number(c.dongle);
  return {
    motorista: { nome: c.motorista.trim() },
    categoria_viagem: c.categoria.trim(),
    combustivel: c.combustivel,
    entrada:
      c.tipo === "boot"
        ? {
            tipo: "boot",
            dongle_id: dongle,
            boot_id: Number(c.bootId),
            ...(c.bootGeneration.trim() ? { boot_generation: Number(c.bootGeneration) } : {}),
          }
        : { tipo: "intervalo", dongle_id: dongle, t0: paraRfc3339(c.t0), t1: paraRfc3339(c.t1) },
  };
}

type Props = { irPara?: (url: string) => void };

export function FormularioProcessar({ irPara = (u) => window.location.assign(u) }: Props) {
  const [c, setC] = useState<Campos>(INICIAL);
  const [erros, setErros] = useState<Erros>({});
  const [enviando, setEnviando] = useState(false);

  const mudar =
    <K extends keyof Campos>(campo: K) =>
    (valor: Campos[K]) =>
      setC((atual) => ({ ...atual, [campo]: valor }));

  async function enviar(ev: FormEvent) {
    ev.preventDefault();
    if (enviando) return;
    const local = validarCampos(c);
    setErros(local);
    if (Object.keys(local).length > 0) return;

    setEnviando(true);
    try {
      const r = await fetch("/api/viagens", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(montarCorpo(c)),
      });
      if (r.status === 202) {
        const corpo = (await r.json()) as CriarViagemResposta;
        irPara(`/viagem/${corpo.viagem_id}`);
        return; // mantém o botão desativado durante a navegação
      }
      if (r.status === 401) {
        irPara(`/entrar?retorno=${encodeURIComponent("/processar")}`);
        return;
      }
      const p = (await r.json().catch(() => null)) as Problema | null;
      if (r.status === 409 && p?.codigo === "motorista_repetido") {
        setErros({
          motorista: `Já existe uma viagem de «${c.motorista.trim()}» na categoria «${c.categoria.trim().toLowerCase()}». Use outro nome.`,
        });
      } else if (r.status === 400 && p?.codigo === "entrada_invalida") {
        setErros({ geral: p.detail ?? "Entrada inválida." });
      } else if (r.status === 404 && p?.codigo === "dongle_desconhecido") {
        setErros({ dongle: "Dongle desconhecido." });
      } else {
        setErros({ geral: "Não foi possível criar a viagem. Tente de novo." });
      }
    } catch {
      setErros({ geral: "Não foi possível criar a viagem. Tente de novo." });
    }
    setEnviando(false);
  }

  const campo = (id: keyof Erros, rotulo: string, entrada: React.ReactNode) => (
    <div className="campo">
      <label htmlFor={`f-${id}`}>{rotulo}</label>
      {entrada}
      {erros[id] ? (
        <p className="campo-erro" id={`erro-${id}`} role="alert">
          {erros[id]}
        </p>
      ) : null}
    </div>
  );
  const ariaErro = (id: keyof Erros) =>
    erros[id] ? { "aria-invalid": true as const, "aria-describedby": `erro-${id}` } : {};

  return (
    <form className="formulario cartao" onSubmit={(e) => void enviar(e)} noValidate>
      {campo(
        "motorista",
        "Motorista",
        <input
          id="f-motorista"
          value={c.motorista}
          maxLength={60}
          autoComplete="off"
          onChange={(e) => mudar("motorista")(e.target.value)}
          {...ariaErro("motorista")}
        />,
      )}
      {campo(
        "categoria",
        "Categoria",
        <input
          id="f-categoria"
          value={c.categoria}
          maxLength={40}
          autoComplete="off"
          onChange={(e) => mudar("categoria")(e.target.value)}
          {...ariaErro("categoria")}
        />,
      )}
      <div className="campo">
        <label htmlFor="f-combustivel">Combustível</label>
        <select
          id="f-combustivel"
          value={c.combustivel}
          onChange={(e) => mudar("combustivel")(e.target.value)}
        >
          {COMBUSTIVEIS.map((o) => (
            <option key={o.id} value={o.id}>
              {o.nome}
            </option>
          ))}
        </select>
      </div>
      {campo(
        "dongle",
        "Dongle",
        <input
          id="f-dongle"
          type="number"
          min={1}
          inputMode="numeric"
          value={c.dongle}
          onChange={(e) => mudar("dongle")(e.target.value)}
          {...ariaErro("dongle")}
        />,
      )}

      <fieldset className="campo">
        <legend>Viagem definida por</legend>
        <div className="opcoes">
          <label className="opcao">
            <input
              type="radio"
              name="tipo"
              checked={c.tipo === "boot"}
              onChange={() => mudar("tipo")("boot")}
            />
            Boot
          </label>
          <label className="opcao">
            <input
              type="radio"
              name="tipo"
              checked={c.tipo === "intervalo"}
              onChange={() => mudar("tipo")("intervalo")}
            />
            Intervalo de tempo
          </label>
        </div>
      </fieldset>

      {c.tipo === "boot" ? (
        <div className="linha-campos">
          {campo(
            "bootId",
            "Boot",
            <input
              id="f-bootId"
              type="number"
              min={1}
              inputMode="numeric"
              value={c.bootId}
              onChange={(e) => mudar("bootId")(e.target.value)}
              {...ariaErro("bootId")}
            />,
          )}
          {campo(
            "bootGeneration",
            "Geração do boot (opcional)",
            <input
              id="f-bootGeneration"
              type="number"
              min={0}
              inputMode="numeric"
              value={c.bootGeneration}
              onChange={(e) => mudar("bootGeneration")(e.target.value)}
              {...ariaErro("bootGeneration")}
            />,
          )}
        </div>
      ) : (
        <div className="linha-campos">
          {campo(
            "t0",
            "Início",
            <input
              id="f-t0"
              type="datetime-local"
              value={c.t0}
              onChange={(e) => mudar("t0")(e.target.value)}
              {...ariaErro("t0")}
            />,
          )}
          {campo(
            "t1",
            "Fim",
            <input
              id="f-t1"
              type="datetime-local"
              value={c.t1}
              onChange={(e) => mudar("t1")(e.target.value)}
              {...ariaErro("t1")}
            />,
          )}
        </div>
      )}

      {erros.geral ? (
        <p className="campo-erro erro-geral" role="alert">
          {erros.geral}
        </p>
      ) : null}
      <Botao type="submit" larguraTotal disabled={enviando}>
        {enviando ? "Enviando…" : "Processar viagem"}
      </Botao>
    </form>
  );
}
