import type { ComponentPropsWithRef, ComponentPropsWithoutRef, ReactNode } from "react";

const LABEL = "type-label text-secondary";

/** A form field: label, control and error stacked. Renders a `fieldset` when given a legend. */
export function Field({ children }: { children: ReactNode }) {
  return <div className="m-0 grid min-w-0 gap-1.5 border-0 p-0">{children}</div>;
}

export function FieldSet({ legend, children }: { legend: string; children: ReactNode }) {
  return (
    <fieldset className="m-0 grid min-w-0 gap-1.5 border-0 p-0">
      <legend className={`${LABEL} p-0`}>{legend}</legend>
      {children}
    </fieldset>
  );
}

export function FieldLabel(props: ComponentPropsWithoutRef<"label">) {
  return <label className={LABEL} {...props} />;
}

const CONTROL =
  "rounded-md border border-strong bg-list px-4 py-0 text-[15px] text-primary aria-invalid:border-danger";

type InputProps = ComponentPropsWithRef<"input"> & { mono?: boolean };

/** Text-like input. Numbers and dates are data, so they use the mono face. */
export function Input({ mono, className, ...rest }: InputProps) {
  const base = `${CONTROL} h-10 ${mono ? "font-mono tabular-nums" : "font-ui"}`;
  return <input className={className ? `${base} ${className}` : base} {...rest} />;
}

export function Select(props: ComponentPropsWithoutRef<"select">) {
  return <select className={`${CONTROL} h-10 font-ui`} {...props} />;
}

/** Radio option: the label wraps the input, in a row. */
export function Radio({ label, ...rest }: { label: string } & ComponentPropsWithoutRef<"input">) {
  return (
    <label className={`flex items-center gap-2 p-0 ${LABEL}`}>
      <input type="radio" className={`${CONTROL} h-auto font-ui`} {...rest} />
      {label}
    </label>
  );
}

/** Two fields side by side; stacked on a narrow screen. */
export function FieldRow({ children }: { children: ReactNode }) {
  return (
    <div className="grid grid-cols-[1fr_1fr] gap-4 phone:grid-cols-[minmax(0,1fr)]">{children}</div>
  );
}

export function FieldError({
  id,
  children,
  box,
}: {
  id?: string;
  children: ReactNode;
  /** Error of the whole form: tinted background. */
  box?: boolean;
}) {
  return (
    <p
      id={id}
      role="alert"
      className={`m-0 text-[13px] text-danger${box ? " rounded-md bg-danger-wash px-4 py-3" : ""}`}
    >
      {children}
    </p>
  );
}
