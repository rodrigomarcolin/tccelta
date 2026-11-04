import type { ButtonHTMLAttributes, ReactNode } from "react";

type Variante = "primario" | "secundario" | "tonal" | "link";

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variante?: Variante;
  icone?: ReactNode;
  larguraTotal?: boolean;
};

/** Botão semântico: a variante carrega significado, não só ênfase. */
export function Botao({
  variante = "primario",
  icone,
  larguraTotal,
  className,
  children,
  type = "button",
  ...resto
}: Props) {
  const classes = ["botao", `botao-${variante}`, larguraTotal ? "botao-total" : "", className ?? ""]
    .filter(Boolean)
    .join(" ");
  return (
    <button type={type} className={classes} {...resto}>
      {icone}
      {children}
    </button>
  );
}
