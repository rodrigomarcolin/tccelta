import type { ReactNode } from "react";

type Props = { wide?: boolean; children: ReactNode };

/**
 * Centered page column: the analysis is wide, the forms are narrow.
 * `print:p-0!` because the breakpoint variants are emitted after `print:`, and print must win.
 */
export function Page({ wide, children }: Props) {
  return (
    <main
      className={
        wide
          ? "mx-auto max-w-[1280px] p-7 tablet:p-5 print:max-w-none print:p-0!"
          : "mx-auto max-w-[640px] p-7"
      }
    >
      {children}
    </main>
  );
}
