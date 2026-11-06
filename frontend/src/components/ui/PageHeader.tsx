import type { ReactNode, Ref } from "react";

type Props = {
  children: ReactNode;
  /** Stays at the top of the window while the page scrolls. */
  sticky?: boolean;
  /** On a phone, a `sticky` header stacks and scrolls away instead of covering the content. */
  stackOnPhone?: boolean;
  ref?: Ref<HTMLElement>;
};

// Negative margins equal to the `Page` padding, so the background covers the whole column.
// `print:*!` because the breakpoint variants are emitted after `print:` (see `Page`).
const STICKY =
  "sticky top-0 z-30 -mx-7 border-b border-hairline bg-screen px-7 pt-2 pb-3 tablet:-mx-5 tablet:px-5 phone:gap-3 print:static print:mx-0! print:border-0 print:p-0!";

/** Title on the left, controls on the right; stacked on a narrow screen (unless `sticky`). */
export function PageHeader({ children, sticky, stackOnPhone, ref }: Props) {
  return (
    <header
      ref={ref}
      className={`mb-7 flex items-start justify-between gap-7 ${sticky ? STICKY : "phone:flex-col"} ${
        stackOnPhone ? "phone:static phone:flex-col" : ""
      }`}
    >
      {children}
    </header>
  );
}

/** Group of title, subtitle and details inside a `PageHeader`. */
export function PageHeaderTitle({ children }: { children: ReactNode }) {
  return <div className="grid gap-1">{children}</div>;
}
