import type { ReactNode } from "react";

type Props = {
  tone: "live" | "warning" | "alert";
  pulse?: boolean;
  children: ReactNode;
};

/** Status pill. The dot pulses only while a request is in flight. */
export function Badge({ tone, pulse, children }: Props) {
  return (
    <span className={`badge badge-${tone}`}>
      <span className={`dot${pulse ? " dot-pulse" : ""}`} aria-hidden="true" />
      {children}
    </span>
  );
}
