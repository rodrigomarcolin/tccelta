import type { ReactNode } from "react";

/** Two equal columns of cards; one column on a narrow screen. */
export function TwoColumns({ children }: { children: ReactNode }) {
  return (
    <div className="grid grid-cols-[repeat(2,minmax(0,1fr))] gap-5 tablet:grid-cols-[minmax(0,1fr)]">
      {children}
    </div>
  );
}
