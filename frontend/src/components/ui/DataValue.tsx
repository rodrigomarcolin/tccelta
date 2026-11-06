import type { ComponentPropsWithoutRef, ReactNode } from "react";

/** Data always in mono, with tabular digits. For cells and terms, use this on their class. */
export const DATA_VALUE = "font-mono tabular-nums";

export function DataValue({ className, ...rest }: ComponentPropsWithoutRef<"span">) {
  return <span className={className ? `${DATA_VALUE} ${className}` : DATA_VALUE} {...rest} />;
}

/** Unit next to a value: smaller and secondary. */
export function Unit({ children }: { children: ReactNode }) {
  return <span className="ml-1 text-[0.85em] text-secondary">{children}</span>;
}
