import type { ButtonHTMLAttributes, ReactNode, Ref } from "react";

type Variant = "primary" | "secondary" | "tonal" | "link" | "danger";
type Size = "default" | "small";

type Props = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: Variant;
  size?: Size;
  icon?: ReactNode;
  fullWidth?: boolean;
  ref?: Ref<HTMLButtonElement>;
};

const BASE =
  "inline-flex items-center justify-center gap-2 rounded-md border type-label no-underline " +
  "cursor-pointer transition-[background,border-color] duration-(--dur-color) ease-value " +
  "disabled:cursor-not-allowed disabled:opacity-50 aria-pressed:border-accent phone:h-11";

const VARIANT: Record<Variant, string> = {
  primary: "border-transparent bg-accent text-accent-on",
  secondary: "border-strong bg-transparent text-primary",
  tonal: "border-accent bg-accent-wash text-accent",
  link: "border-transparent bg-transparent text-accent",
  danger: "border-transparent bg-danger text-accent-on",
};

// The link variant is shorter and narrower; `small` wins over both, as it did before.
// `type-label` is a `font` shorthand and is emitted after the size utilities, so the 14px needs
// `!` to win over its 13px. Padding is explicit because the old button had no vertical padding.
const SIZE = "h-10 px-5 py-0 text-[14px]!";
const LINK_SIZE = "h-8 px-2 py-0 text-[14px]!";
// 13px is what `type-label` already gives.
const SMALL_SIZE = "h-[30px] px-3 py-0";

type ClassOptions = { variant?: Variant; size?: Size; fullWidth?: boolean; className?: string };

/** Classes of a button; also used by links that look like buttons. */
export function buttonClass({
  variant = "primary",
  size = "default",
  fullWidth,
  className,
}: ClassOptions = {}): string {
  const sizing = size === "small" ? SMALL_SIZE : variant === "link" ? LINK_SIZE : SIZE;
  return [BASE, VARIANT[variant], sizing, fullWidth ? "w-full" : "", className ?? ""]
    .filter(Boolean)
    .join(" ");
}

/** Semantic button: the variant carries meaning, not just emphasis. */
export function Button({
  variant = "primary",
  size = "default",
  icon,
  fullWidth,
  className,
  children,
  type = "button",
  ...rest
}: Props) {
  return (
    <button type={type} className={buttonClass({ variant, size, fullWidth, className })} {...rest}>
      {icon}
      {children}
    </button>
  );
}
