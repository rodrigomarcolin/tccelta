import type { ReactNode } from "react";

type Tone = "live" | "warning" | "alert";

type Props = {
  tone: Tone;
  pulse?: boolean;
  children: ReactNode;
};

const TONE: Record<Tone, string> = {
  live: "bg-accent-wash text-accent",
  warning: "bg-warning-wash text-warning",
  alert: "bg-danger-wash text-danger",
};

// The glow is the only shadow in the interface: it marks the "live" dot.
const DOT: Record<Tone, string> = {
  live: "shadow-(--glow-cyan)",
  warning: "",
  alert: "",
};

/** Status pill. The dot pulses only while a request is in flight. */
export function Badge({ tone, pulse, children }: Props) {
  const dot = [
    "size-2 rounded-pill bg-current",
    DOT[tone],
    pulse ? "animate-breathe motion-reduce:animate-none" : "",
  ]
    .filter(Boolean)
    .join(" ");
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-sm px-3 py-1 type-overline tracking-[1px] uppercase ${TONE[tone]}`}
    >
      <span className={dot} aria-hidden="true" />
      {children}
    </span>
  );
}
