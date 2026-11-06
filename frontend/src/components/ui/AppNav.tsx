import Link from "next/link";
import { buttonClass } from "./Button";
import { ThemeToggle } from "./Theme";

type Props = { current?: "viagens" | "nova" };

const ITEMS = [
  { id: "viagens", href: "/viagens", label: "Viagens" },
  { id: "nova", href: "/processar", label: "Nova viagem" },
] as const;

/** Main navigation of the app: the trip list and the form. Not printed. */
export function AppNav({ current, hiddenOnPhone }: Props & { hiddenOnPhone?: boolean }) {
  return (
    <nav
      aria-label="Principal"
      className={`flex gap-2 print:hidden ${hiddenOnPhone ? "phone:hidden" : ""}`}
    >
      {ITEMS.map((item) => (
        <Link
          key={item.id}
          href={item.href}
          aria-current={current === item.id ? "page" : undefined}
          className={buttonClass({ variant: current === item.id ? "tonal" : "secondary" })}
        >
          {item.label}
        </Link>
      ))}
    </nav>
  );
}

/**
 * Navigation and theme toggle side by side, for the right side of a `PageHeader`.
 * `navHiddenOnPhone`: for a header that stays beside its title and already links back.
 */
export function HeaderControls({
  current,
  navHiddenOnPhone,
}: Props & { navHiddenOnPhone?: boolean }) {
  return (
    <div className="flex shrink-0 flex-wrap items-center justify-end gap-3 phone:justify-start print:hidden">
      <AppNav current={current} hiddenOnPhone={navHiddenOnPhone} />
      <ThemeToggle />
    </div>
  );
}
