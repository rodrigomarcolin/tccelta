import type { ReactNode } from "react";

/** Title on the left, controls on the right; stacked on a narrow screen. */
export function PageHeader({ children }: { children: ReactNode }) {
  return (
    <header className="mb-7 flex items-start justify-between gap-7 phone:flex-col">
      {children}
    </header>
  );
}

/** Group of title, subtitle and details inside a `PageHeader`. */
export function PageHeaderTitle({ children }: { children: ReactNode }) {
  return <div className="grid gap-1">{children}</div>;
}
