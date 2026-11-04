import type { ReactNode } from "react";

type Props = { children: ReactNode; flush?: boolean };

/** Small uppercase heading. `flush` removes the space above, for the first heading of a card. */
export function Overline({ children, flush }: Props) {
  return (
    <h3
      className={`${flush ? "mt-0" : "mt-5"} mb-2 type-overline tracking-overline text-secondary uppercase`}
    >
      {children}
    </h3>
  );
}
